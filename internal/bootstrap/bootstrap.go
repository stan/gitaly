package bootstrap

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
)

type Bootstrap struct {
	GracefulStop chan struct{}

	upgrader *tableflip.Upgrader
	wg       sync.WaitGroup
	errChan  chan error
	starters []Starter
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
func New(pidFile string, upgradesEnabled bool) (*Bootstrap, error) {
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

	gracefulStopCh := make(chan struct{})
	go func() { <-upg.Exit(); close(gracefulStopCh) }()

	return &Bootstrap{
		upgrader:     upg,
		GracefulStop: gracefulStopCh,
	}, nil
}

type ListenFunc func(net, addr string) (net.Listener, error)

type Starter func(ListenFunc, chan<- error) error

func (b *Bootstrap) IsFirstBoot() bool { return !b.upgrader.HasParent() }

func (b *Bootstrap) RegisterStarter(starter Starter) {
	b.starters = append(b.starters, starter)
}

func (b *Bootstrap) Start() error {
	b.errChan = make(chan error, len(b.starters))

	for _, start := range b.starters {
		errCh := make(chan error)

		if err := start(b.upgrader.Fds.Listen, errCh); err != nil {
			return err
		}

		b.wg.Add(1)
		go func(errCh chan error) {
			err := <-errCh
			b.wg.Done()
			b.errChan <- err
		}(errCh)
	}

	return nil
}

func (b *Bootstrap) Wait() error {
	signals := []os.Signal{syscall.SIGTERM, syscall.SIGINT}
	immediateShutdown := make(chan os.Signal, len(signals))
	signal.Notify(immediateShutdown, signals...)

	if err := b.upgrader.Ready(); err != nil {
		return err
	}

	var err error
	select {
	case <-b.upgrader.Exit():
		// this is the old process and a graceful upgrade is in progress
		// the new process signaled its readiness and we started a graceful stop
		// however no further upgrades can be started until this process is running
		// we set a grace period and then we force a termination.
		b.waitGracePeriod(immediateShutdown)

		err = fmt.Errorf("graceful upgrade")
	case s := <-immediateShutdown:
		err = fmt.Errorf("received signal %q", s)
	case err = <-b.errChan:
	}

	return err
}

func (b *Bootstrap) waitGracePeriod(kill <-chan os.Signal) {
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
