package server

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
	gitalyauth "gitlab.com/gitlab-org/gitaly/auth"
	"gitlab.com/gitlab-org/gitaly/internal/config"
	"gitlab.com/gitlab-org/gitaly/internal/git"
	"gitlab.com/gitlab-org/gitaly/internal/server/auth"
	"gitlab.com/gitlab-org/gitaly/internal/testhelper"
	"gitlab.com/gitlab-org/gitaly/internal/version"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func TestGitalyServerInfo(t *testing.T) {
	server, serverSocketPath := runServer(t)
	defer server.Stop()

	client, conn := newServerClient(t, serverSocketPath)
	defer conn.Close()

	ctx, cancel := testhelper.Context()
	defer cancel()

	// Setup storage paths
	testStorages := []config.Storage{
		{Name: "default", Path: testhelper.GitlabTestStoragePath()},
		{Name: "broken", Path: "/does/not/exist"},
	}
	defer func(oldStorages []config.Storage) {
		config.Config.Storages = oldStorages
	}(config.Config.Storages)
	config.Config.Storages = testStorages

	c, err := client.ServerInfo(ctx, &gitalypb.ServerInfoRequest{})
	require.NoError(t, err)

	require.Equal(t, version.GetVersion(), c.GetServerVersion())

	gitVersion, err := git.Version()
	require.NoError(t, err)
	require.Equal(t, gitVersion, c.GetGitVersion())

	require.Len(t, c.GetStorageStatuses(), len(testStorages))
	require.True(t, c.GetStorageStatuses()[0].Readable)
	require.True(t, c.GetStorageStatuses()[0].Writeable)
	require.NotEmpty(t, c.GetStorageStatuses()[0].FsType)

	require.False(t, c.GetStorageStatuses()[1].Readable)
	require.False(t, c.GetStorageStatuses()[1].Writeable)
}

func runServer(t *testing.T) (*grpc.Server, string) {
	streamInt := []grpc.StreamServerInterceptor{auth.StreamServerInterceptor()}
	unaryInt := []grpc.UnaryServerInterceptor{auth.UnaryServerInterceptor()}

	server := testhelper.NewTestGrpcServer(t, streamInt, unaryInt)
	serverSocketPath := testhelper.GetTemporaryGitalySocketFileName()

	listener, err := net.Listen("unix", serverSocketPath)
	if err != nil {
		t.Fatal(err)
	}

	gitalypb.RegisterServerServiceServer(server, NewServer())
	reflection.Register(server)

	go server.Serve(listener)

	return server, "unix://" + serverSocketPath
}

func newServerClient(t *testing.T, serverSocketPath string) (gitalypb.ServerServiceClient, *grpc.ClientConn) {
	connOpts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithPerRPCCredentials(gitalyauth.RPCCredentials(testhelper.RepositoryAuthToken)),
	}
	conn, err := grpc.Dial(serverSocketPath, connOpts...)
	if err != nil {
		t.Fatal(err)
	}

	return gitalypb.NewServerServiceClient(conn), conn
}
