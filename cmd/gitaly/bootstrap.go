package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cloudflare/tableflip"
	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitaly/internal/config"
	"gitlab.com/gitlab-org/gitaly/internal/connectioncounter"
	"gitlab.com/gitlab-org/gitaly/internal/rubyserver"
	"gitlab.com/gitlab-org/gitaly/internal/server"
	"google.golang.org/grpc"
)

type bootstrap struct {
	*tableflip.Upgrader

	insecureListeners []net.Listener
	secureListeners   []net.Listener

	serversErrors chan error
	wg            sync.WaitGroup
}

// newBootstrap performs tableflip initialization
//
// first boot:
// * gitaly starts as usual, we will refer to it as p1
// * newBootstrap will build a tableflip.Upgrader, we will refer to it as upg
// * sockets and files must be opened with upg.Fds
// * p1 will trap SIGHUP and invoke upg.Upgrade()
// * when ready to accept incoming connections p1 will call upg.Ready()
// * upg.Exit() channel will be closed when an upgrades completed successfully and the process must terminate
//
// graceful upgrade:
// * user replaces gitaly binary and/or config file
// * user sends SIGHUP to p1
// * p1 will fork and exec the new gitaly, we will refer to it as p2
// * from now on p1 will ignore other SIGHUP
// * if p2 terminates with a non-zero exit code, SIGHUP handling will be restored
// * p2 will follow the "first boot" sequence but upg.Fds will provide sockets and files from p1, when available
// * when p2 invokes upg.Ready() all the shared file descriptors not claimed by p2 will be closed
// * upg.Exit() channel in p1 will be closed now and p1 can gracefully terminate already accepted connections
// * upgrades cannot starts again if p1 and p2 are both running, an hard termination should be scheduled to overcome
//   freezes during a graceful shutdown
func newBootstrap(pidFile string, upgradesEnabled bool) (*bootstrap, error) {
	// PIDFile is optional, if provided tableflip will keep it updated
	upg, err := tableflip.New(tableflip.Options{PIDFile: pidFile})
	if err != nil {
		return nil, err
	}

	if upgradesEnabled {
		go func() {
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGHUP)

			for range sig {
				err := upg.Upgrade()
				if err != nil {
					log.WithError(err).Error("Upgrade failed")
					continue
				}

				log.Info("Upgrade succeeded")
			}
		}()
	}

	return &bootstrap{Upgrader: upg}, nil
}

func (b *bootstrap) listen() error {
	if socketPath := config.Config.SocketPath; socketPath != "" {
		l, err := b.createUnixListener(socketPath)
		if err != nil {
			return err
		}

		log.WithField("address", socketPath).Info("listening on unix socket")
		b.insecureListeners = append(b.insecureListeners, l)
	}

	if addr := config.Config.ListenAddr; addr != "" {
		l, err := b.Fds.Listen("tcp", addr)
		if err != nil {
			return err
		}

		log.WithField("address", addr).Info("listening at tcp address")
		b.insecureListeners = append(b.insecureListeners, connectioncounter.New("tcp", l))
	}

	if addr := config.Config.TLSListenAddr; addr != "" {
		tlsListener, err := b.Fds.Listen("tcp", addr)
		if err != nil {
			return err
		}

		b.secureListeners = append(b.secureListeners, connectioncounter.New("tls", tlsListener))
	}

	cnt := len(b.insecureListeners) + len(b.secureListeners)
	b.serversErrors = make(chan error, cnt)
	b.wg.Add(cnt)

	return nil
}

func (b *bootstrap) prometheusListener() (net.Listener, error) {
	log.WithField("address", config.Config.PrometheusListenAddr).Info("starting prometheus listener")

	return b.Fds.Listen("tcp", config.Config.PrometheusListenAddr)
}

func (b *bootstrap) run() {
	signals := []os.Signal{syscall.SIGTERM, syscall.SIGINT}
	done := make(chan os.Signal, len(signals))
	signal.Notify(done, signals...)

	ruby, err := rubyserver.Start()
	if err != nil {
		log.WithError(err).Error("start ruby server")
		return
	}
	defer ruby.Stop()

	if len(b.insecureListeners) > 0 {
		insecureServer := server.NewInsecure(ruby)
		defer insecureServer.Stop()

		b.serve(insecureServer, b.insecureListeners)
	}

	if len(b.secureListeners) > 0 {
		secureServer := server.NewSecure(ruby)
		defer secureServer.Stop()

		b.serve(secureServer, b.secureListeners)
	}

	if err := b.Ready(); err != nil {
		log.WithError(err).Error("incomplete bootstrap")
		return
	}

	select {
	case <-b.Exit():
		// this is the old process and a graceful upgrade is in progress
		// the new process signaled its readiness and we started a graceful stop
		// however no further upgrades can be started until this process is running
		// we set a grace period and then we force a termination.
		b.waitGracePeriod(done)

		err = fmt.Errorf("graceful upgrade")
	case s := <-done:
		err = fmt.Errorf("received signal %q", s)
	case err = <-b.serversErrors:
	}

	log.WithError(err).Error("terminating")
}

func (b *bootstrap) waitGracePeriod(kill <-chan os.Signal) {
	log.WithField("graceful_restart_timeout", config.Config.GracefulRestartTimeout).Warn("starting grace period")

	allServersDone := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(allServersDone)
	}()

	select {
	case <-time.After(config.Config.GracefulRestartTimeout):
		log.Error("old process stuck on termination. Grace period expired.")
	case <-kill:
		log.Error("force shutdown")
	case <-allServersDone:
		log.Info("graceful stop completed")
	}
}

func (b *bootstrap) createUnixListener(socketPath string) (net.Listener, error) {
	if !b.HasParent() {
		// During an update the unix socket exists and if we delete it tableflip will not create a new one
		if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}

	l, err := b.Fds.Listen("unix", socketPath)
	return connectioncounter.New("unix", l), err
}

func (b *bootstrap) serve(server *grpc.Server, listeners []net.Listener) {
	go func() {
		<-b.Exit()

		server.GracefulStop()
	}()

	for _, listener := range listeners {
		// Must pass the listener as a function argument because there is a race
		// between 'go' and 'for'.
		go func(l net.Listener) {
			b.serversErrors <- server.Serve(l)
			b.wg.Done()
		}(listener)
	}
}
