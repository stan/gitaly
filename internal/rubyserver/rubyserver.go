package rubyserver

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitaly/internal/command"
	"gitlab.com/gitlab-org/gitaly/internal/config"
	"gitlab.com/gitlab-org/gitaly/internal/git/hooks"
	"gitlab.com/gitlab-org/gitaly/internal/helper"
	"gitlab.com/gitlab-org/gitaly/internal/rubyserver/balancer"
	"gitlab.com/gitlab-org/gitaly/internal/supervisor"
	"gitlab.com/gitlab-org/gitaly/internal/version"
	"gitlab.com/gitlab-org/gitaly/streamio"
	grpccorrelation "gitlab.com/gitlab-org/labkit/correlation/grpc"
	grpctracing "gitlab.com/gitlab-org/labkit/tracing/grpc"
	"google.golang.org/grpc"
)

var (
	socketDir string

	lazyInit sync.Once

	// ConnectTimeout is the timeout for establishing a connection to the gitaly-ruby process.
	ConnectTimeout = 40 * time.Second
)

func init() {
	timeout64, err := strconv.ParseInt(os.Getenv("GITALY_RUBY_CONNECT_TIMEOUT"), 10, 32)
	if err == nil && timeout64 > 0 {
		ConnectTimeout = time.Duration(timeout64) * time.Second
	}
}

// prepareSocketPath will create a socket path unless one is set in the configuration
// managed by the administrator. The socket path must but short as listen(2) fails
// on long socket paths. This is either achieved by using a relative directory
// from the Gitaly binary. Or we hope that ioutil.TempDir creates a directory
// that's not too deep.
// A directory is needed, not a tempfile, because we will later want to set its
// permissions to 0700. The permission change is done in the Ruby child process
func prepareSocketPath() {
	var err error

	socketDir = config.Config.InternalSocketDir
	if socketDir == "" {
		socketDir, err = ioutil.TempDir("", "gitaly-ruby")
		if err != nil {
			log.Fatalf("create ruby server socket directory: %v", err)
		}

	}
}

func socketPath(id int) string {
	if socketDir == "" {
		panic("socketDir is not set")
	}

	return path.Join(filepath.Clean(socketDir), fmt.Sprintf("socket.%d", id))
}

// Server represents a gitaly-ruby helper process.
type Server struct {
	workers      []*worker
	clientConnMu sync.Mutex
	clientConn   *grpc.ClientConn
}

// Stop shuts down the gitaly-ruby helper process and cleans up resources.
func (s *Server) Stop() {
	if s != nil {
		s.clientConnMu.Lock()
		defer s.clientConnMu.Unlock()
		if s.clientConn != nil {
			s.clientConn.Close()
		}

		for _, w := range s.workers {
			w.stopMonitor()
			w.Process.Stop()
		}
	}

	if socketDir != "" {
		os.RemoveAll(socketDir)
	}
}

// Start spawns the Ruby server.
func Start() (*Server, error) {
	lazyInit.Do(prepareSocketPath)

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	cfg := config.Config
	env := append(
		os.Environ(),
		"GITALY_RUBY_GIT_BIN_PATH="+command.GitPath(),
		fmt.Sprintf("GITALY_RUBY_WRITE_BUFFER_SIZE=%d", streamio.WriteBufferSize),
		fmt.Sprintf("GITALY_RUBY_MAX_COMMIT_OR_TAG_MESSAGE_SIZE=%d", helper.MaxCommitOrTagMessageSize),
		"GITALY_RUBY_GITLAB_SHELL_PATH="+cfg.GitlabShell.Dir,
		"GITALY_RUBY_GITALY_BIN_DIR="+cfg.BinDir,
		"GITALY_VERSION="+version.GetVersion(),
		"GITALY_GIT_HOOKS_DIR="+hooks.Path(),
	)

	env = append(env, command.GitEnv...)

	if dsn := cfg.Logging.RubySentryDSN; dsn != "" {
		env = append(env, "SENTRY_DSN="+dsn)
	}

	if sentryEnvironment := cfg.Logging.SentryEnvironment; sentryEnvironment != "" {
		env = append(env, "SENTRY_ENVIRONMENT="+sentryEnvironment)
	}

	gitalyRuby := path.Join(cfg.Ruby.Dir, "bin/gitaly-ruby")

	numWorkers := cfg.Ruby.NumWorkers
	balancer.ConfigureBuilder(numWorkers, 0)

	s := &Server{}
	for i := 0; i < numWorkers; i++ {
		name := fmt.Sprintf("gitaly-ruby.%d", i)
		socketPath := socketPath(i)

		// Use 'ruby-cd' to make sure gitaly-ruby has the same working directory
		// as the current process. This is a hack to sort-of support relative
		// Unix socket paths.
		args := []string{"bundle", "exec", "bin/ruby-cd", wd, gitalyRuby, strconv.Itoa(os.Getpid()), socketPath}

		events := make(chan supervisor.Event)
		check := func() error { return ping(socketPath) }
		p, err := supervisor.New(name, env, args, cfg.Ruby.Dir, cfg.Ruby.MaxRSS, events, check)
		if err != nil {
			return nil, err
		}

		s.workers = append(s.workers, newWorker(p, socketPath, events, false))
	}

	return s, nil
}

// CommitServiceClient returns a CommitServiceClient instance that is
// configured to connect to the running Ruby server. This assumes Start()
// has been called already.
func (s *Server) CommitServiceClient(ctx context.Context) (gitalypb.CommitServiceClient, error) {
	conn, err := s.getConnection(ctx)
	return gitalypb.NewCommitServiceClient(conn), err
}

// DiffServiceClient returns a DiffServiceClient instance that is
// configured to connect to the running Ruby server. This assumes Start()
// has been called already.
func (s *Server) DiffServiceClient(ctx context.Context) (gitalypb.DiffServiceClient, error) {
	conn, err := s.getConnection(ctx)
	return gitalypb.NewDiffServiceClient(conn), err
}

// RefServiceClient returns a RefServiceClient instance that is
// configured to connect to the running Ruby server. This assumes Start()
// has been called already.
func (s *Server) RefServiceClient(ctx context.Context) (gitalypb.RefServiceClient, error) {
	conn, err := s.getConnection(ctx)
	return gitalypb.NewRefServiceClient(conn), err
}

// OperationServiceClient returns a OperationServiceClient instance that is
// configured to connect to the running Ruby server. This assumes Start()
// has been called already.
func (s *Server) OperationServiceClient(ctx context.Context) (gitalypb.OperationServiceClient, error) {
	conn, err := s.getConnection(ctx)
	return gitalypb.NewOperationServiceClient(conn), err
}

// RepositoryServiceClient returns a RefServiceClient instance that is
// configured to connect to the running Ruby server. This assumes Start()
// has been called already.
func (s *Server) RepositoryServiceClient(ctx context.Context) (gitalypb.RepositoryServiceClient, error) {
	conn, err := s.getConnection(ctx)
	return gitalypb.NewRepositoryServiceClient(conn), err
}

// WikiServiceClient returns a WikiServiceClient instance that is
// configured to connect to the running Ruby server. This assumes Start()
// has been called already.
func (s *Server) WikiServiceClient(ctx context.Context) (gitalypb.WikiServiceClient, error) {
	conn, err := s.getConnection(ctx)
	return gitalypb.NewWikiServiceClient(conn), err
}

// ConflictsServiceClient returns a ConflictsServiceClient instance that is
// configured to connect to the running Ruby server. This assumes Start()
// has been called already.
func (s *Server) ConflictsServiceClient(ctx context.Context) (gitalypb.ConflictsServiceClient, error) {
	conn, err := s.getConnection(ctx)
	return gitalypb.NewConflictsServiceClient(conn), err
}

// RemoteServiceClient returns a RemoteServiceClient instance that is
// configured to connect to the running Ruby server. This assumes Start()
// has been called already.
func (s *Server) RemoteServiceClient(ctx context.Context) (gitalypb.RemoteServiceClient, error) {
	conn, err := s.getConnection(ctx)
	return gitalypb.NewRemoteServiceClient(conn), err
}

// BlobServiceClient returns a BlobServiceClient instance that is
// configured to connect to the running Ruby server. This assumes Start()
// has been called already.
func (s *Server) BlobServiceClient(ctx context.Context) (gitalypb.BlobServiceClient, error) {
	conn, err := s.getConnection(ctx)
	return gitalypb.NewBlobServiceClient(conn), err
}

func (s *Server) getConnection(ctx context.Context) (*grpc.ClientConn, error) {
	s.clientConnMu.Lock()
	conn := s.clientConn
	s.clientConnMu.Unlock()

	if conn != nil {
		return conn, nil
	}

	return s.createConnection(ctx)
}

func (s *Server) createConnection(ctx context.Context) (*grpc.ClientConn, error) {
	s.clientConnMu.Lock()
	defer s.clientConnMu.Unlock()

	if conn := s.clientConn; conn != nil {
		return conn, nil
	}

	dialCtx, cancel := context.WithTimeout(ctx, ConnectTimeout)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, balancer.Scheme+":///gitaly-ruby", dialOptions()...)
	if err != nil {
		return nil, err
	}

	s.clientConn = conn
	return s.clientConn, nil
}

func dialOptions() []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithBlock(), // With this we get retries. Without, connections fail fast.
		grpc.WithInsecure(),
		// Use a custom dialer to ensure that we don't experience
		// issues in environments that have proxy configurations
		// https://gitlab.com/gitlab-org/gitaly/merge_requests/1072#note_140408512
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
		grpc.WithUnaryInterceptor(
			grpc_middleware.ChainUnaryClient(
				grpc_prometheus.UnaryClientInterceptor,
				grpctracing.UnaryClientTracingInterceptor(),
				grpccorrelation.UnaryClientCorrelationInterceptor(),
			),
		),
		grpc.WithStreamInterceptor(
			grpc_middleware.ChainStreamClient(
				grpc_prometheus.StreamClientInterceptor,
				grpctracing.StreamClientTracingInterceptor(),
				grpccorrelation.StreamClientCorrelationInterceptor(),
			),
		),
	}
}
