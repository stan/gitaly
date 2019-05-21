package bootstrap

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"testing"
	"time"

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
	s := http.Server{
		Handler: http.NotFoundHandler(),
	}
	defer s.Close()

	u := &mockUpgrader{}
	b, err := _new(u, net.Listen, false)
	require.NoError(t, err)

	listeners := make([]net.Listener, 0, 2)
	start := func(network, address string) Starter {
		return func(listen ListenFunc, errors chan<- error) error {
			l, err := listen(network, address)
			if err != nil {
				return err
			}
			listeners = append(listeners, l)

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

	addr := listeners[0].Addr()
	r, err := http.Get(fmt.Sprintf("http://%s/", addr.String()))
	require.NoError(t, err)
	r.Body.Close()
	require.Equal(t, 404, r.StatusCode)

	time.AfterFunc(500*time.Millisecond, func() {
		require.NoError(t, listeners[0].Close(), "Closing first listener")
	})

	err = b.Wait()
	require.Error(t, err)
	require.Contains(t, err.Error(), "use of closed network connection")
}
