package bootstrap

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"syscall"
	"testing"
	"time"

	"gitlab.com/gitlab-org/gitaly/internal/config"

	"github.com/stretchr/testify/require"
)

type mockUpgrader struct {
	exit                     chan struct{}
	hasParent                bool
	readyError, upgradeError error
}

func (m *mockUpgrader) Exit() <-chan struct{} {
	return m.exit
}

func (m *mockUpgrader) HasParent() bool {
	return m.hasParent
}

func (m *mockUpgrader) Ready() error {
	return m.readyError
}

func (m *mockUpgrader) Upgrade() error {
	// to upgrade we close the exit channel
	close(m.exit)
	return m.upgradeError
}

func TestCreateUnixListener(t *testing.T) {
	socketPath := path.Join(os.TempDir(), "gitaly-test-unix-socket")
	// simulate a dangling socket
	if err := os.Remove(socketPath); err != nil {
		require.True(t, os.IsNotExist(err), "cannot delete dangling socket: %v", err)
	}

	file, err := os.OpenFile(socketPath, os.O_CREATE, 0755)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	require.NoError(t, ioutil.WriteFile(socketPath, nil, 0755))

	listen := func(network, addr string) (net.Listener, error) {
		require.Equal(t, "unix", network)
		require.Equal(t, socketPath, addr)

		return net.Listen(network, addr)
	}
	u := &mockUpgrader{}
	b, err := _new(u, listen, false)
	require.NoError(t, err)

	l, err := b.listen("unix", socketPath)
	require.NoError(t, err, "failed to bind on fist boot")
	require.NoError(t, l.Close())

	// simulate binding during an upgrade
	u.hasParent = true
	l, err = b.listen("unix", socketPath)
	require.NoError(t, err, "failed to bind on upgrade")
	require.NoError(t, l.Close())
}

func TestImmediateTerminationOnSocketError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	b, _, listeners := makeBootstrap(ctx, t)

	time.AfterFunc(500*time.Millisecond, func() {
		require.NoError(t, listeners["tcp"].Close(), "Closing first listener")
	})

	err := b.Wait()
	require.Error(t, err)
	require.Contains(t, err.Error(), "use of closed network connection")
}

func TestImmediateTerminationOnSignal(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGINT} {
		t.Run(sig.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()
			b, url, _ := makeBootstrap(ctx, t)

			go slowRequest(t, url, 3*time.Minute, true)

			time.AfterFunc(500*time.Millisecond, func() {
				self, err := os.FindProcess(os.Getpid())
				require.NoError(t, err)

				require.NoError(t, self.Signal(sig))
			})

			err := b.Wait()
			require.Error(t, err)
			require.Contains(t, err.Error(), "received signal")
			require.Contains(t, err.Error(), sig.String())
		})
	}
}

func TestImmediateTerminationGracePeriod(t *testing.T) {
	defer func(oldVal time.Duration) {
		config.Config.GracefulRestartTimeout = oldVal
	}(config.Config.GracefulRestartTimeout)
	config.Config.GracefulRestartTimeout = 10 * time.Second

	basicTest := func(t *testing.T, graceful bool) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer cancel()
		b, url, _ := makeBootstrap(ctx, t)

		go slowRequest(t, url, 2*time.Second, graceful)

		time.AfterFunc(300*time.Millisecond, func() {
			b.upgrader.Upgrade()
		})

		err := b.Wait()
		require.Error(t, err)
		require.Contains(t, err.Error(), "graceful upgrade")
	}

	t.Run("complete", func(t *testing.T) {
		basicTest(t, true)
	})

	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGINT} {
		t.Run(sig.String(), func(t *testing.T) {
			time.AfterFunc(700*time.Millisecond, func() {
				self, err := os.FindProcess(os.Getpid())
				require.NoError(t, err)

				require.NoError(t, self.Signal(sig))
			})

			basicTest(t, false)
		})
	}

	t.Run("stuck", func(t *testing.T) {
		basicTest(t, true)
	})
}

func slowRequest(t *testing.T, url string, duration time.Duration, failure bool) {
	r, err := http.Get(fmt.Sprintf("%sslow?seconds=%d", url, int(duration.Seconds())))
	if failure {
		require.Error(t, err)
	} else {
		require.NoError(t, err)
		r.Body.Close()
	}
}

func makeBootstrap(ctx context.Context, t *testing.T) (*Bootstrap, string, map[string]net.Listener) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		sec, err := strconv.Atoi(r.URL.Query().Get("seconds"))
		require.NoError(t, err)
		time.Sleep(time.Duration(sec) * time.Second)

		w.WriteHeader(200)
	})

	s := http.Server{Handler: mux}
	u := &mockUpgrader{exit: make(chan struct{})}

	go func() {
		select {
		case <-ctx.Done():
			close(u.exit)
		case <-u.exit:
			s.Shutdown(ctx)
		}
	}()

	b, err := _new(u, net.Listen, false)
	require.NoError(t, err)

	listeners := make(map[string]net.Listener)
	start := func(network, address string) Starter {
		return func(listen ListenFunc, errors chan<- error) error {
			l, err := listen(network, address)
			if err != nil {
				return err
			}
			listeners[network] = l

			go func() {
				errors <- s.Serve(l)
			}()

			return nil
		}
	}

	for network, address := range map[string]string{
		"tcp":  "127.0.0.1:0",
		"unix": path.Join(os.TempDir(), "gitaly-test-unix-socket"),
	} {
		b.RegisterStarter(start(network, address))
	}

	require.NoError(t, b.Start())
	require.Equal(t, 2, len(listeners))

	// test connection
	addr := listeners["tcp"].Addr()
	url := fmt.Sprintf("http://%s/", addr.String())

	r, err := http.Get(url)
	require.NoError(t, err)
	r.Body.Close()
	require.Equal(t, 200, r.StatusCode)

	return b, url, listeners
}
