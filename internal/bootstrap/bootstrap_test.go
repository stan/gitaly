package bootstrap

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// b is global because tableflip do not allow to init more than one Upgrader per process
var b *Bootstrap
var socketPath = path.Join(os.TempDir(), "test-unix-socket")

// TestMain helps testing bootstrap.
// When invoked directly it behaves like a normal go test, but if a test performs an upgrade the children will
// avoid the test suite and start a pid HTTP server on socketPath
func TestMain(m *testing.M) {
	var err error
	b, err = New("", true)
	if err != nil {
		panic(err)
	}

	if b.IsFirstBoot() {
		os.Exit(m.Run())
	}

	// this is a test suite that triggered an upgrade, we are in the children here
	l, err := b.createUnixListener(socketPath)
	if err != nil {
		panic(err)
	}

	if err := b.Ready(); err != nil {
		panic(err)
	}

	done := make(chan struct{})
	srv := startPidServer(done, l)

	select {
	case <-done:
	//no op
	case <-time.After(2 * time.Minute):
		srv.Close()
		panic("safeguard against zombie process")
	}
}

func TestCreateUnixListener(t *testing.T) {
	// simulate a dangling socket
	if err := os.Remove(socketPath); err != nil {
		require.True(t, os.IsNotExist(err), "cannot delete dangling socket: %v", err)
	}

	file, err := os.OpenFile(socketPath, os.O_CREATE, 0755)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	require.NoError(t, ioutil.WriteFile(socketPath, nil, 0755))

	l, err := b.createUnixListener(socketPath)
	require.NoError(t, err)

	done := make(chan struct{})
	srv := startPidServer(done, l)
	defer srv.Close()

	require.NoError(t, b.Ready(), "not ready")

	myPid, err := askPid()
	require.NoError(t, err)
	require.Equal(t, os.Getpid(), myPid)

	// we trigger an upgrade and wait for children readiness
	require.NoError(t, b.Upgrade(), "upgrade failed")
	<-b.Exit()
	require.NoError(t, srv.Close())
	<-done

	childPid, err := askPid()
	require.NoError(t, err)
	require.NotEqual(t, os.Getpid(), childPid, "this request must be handled by the children")
}

func askPid() (int, error) {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	response, err := client.Get("http://unix")
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()

	pid, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(string(pid))
}

// startPidServer starts an HTTP server that returns the current PID, if running on a children it will kill itself after serving
// the first client
func startPidServer(done chan<- struct{}, l net.Listener) *http.Server {
	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, fmt.Sprint(os.Getpid()))

		if b.HasParent() {
			time.AfterFunc(1*time.Second, func() { srv.Close() })
		}
	})

	go func() {
		if err := srv.Serve(l); err != http.ErrServerClosed {
			fmt.Printf("Serve error: %v", err)
		}
		close(done)
	}()

	return srv
}
