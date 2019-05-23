package repository

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitaly/internal/config"
	"gitlab.com/gitlab-org/gitaly/internal/testhelper"
	"google.golang.org/grpc/codes"
)

func TestSuccessfulRepositorySizeRequest(t *testing.T) {
	server, serverSocketPath := runRepoServer(t)
	defer server.Stop()

	client, conn := newRepositoryClient(t, serverSocketPath)
	defer conn.Close()

	storageName := "default"
	storagePath, found := config.StoragePath(storageName)
	if !found {
		t.Fatalf("No %q storage was found", storageName)
	}

	repoCopyPath := path.Join(storagePath, "fixed-size-repo.git")
	testhelper.MustRunCommand(t, nil, "cp", "-R", "testdata/fixed-size-repo.git", repoCopyPath)
	// run `sync` because some filesystems (e.g. ZFS and BTRFS) do lazy-writes
	// which leads to `du` returning 0 bytes used until it's finally written to disk.
	testhelper.MustRunCommand(t, nil, "sync")
	defer os.RemoveAll(repoCopyPath)

	request := &gitalypb.RepositorySizeRequest{
		Repository: &gitalypb.Repository{
			StorageName:  storageName,
			RelativePath: "fixed-size-repo.git",
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	response, err := client.RepositorySize(ctx, request)
	require.NoError(t, err)
	// We can't test for an exact size because it will be different for systems with different sector sizes,
	// so we settle for anything greater than zero.
	require.True(t, response.Size > 0, "size must be greater than zero")
}

func TestFailedRepositorySizeRequest(t *testing.T) {
	server, serverSocketPath := runRepoServer(t)
	defer server.Stop()

	client, conn := newRepositoryClient(t, serverSocketPath)
	defer conn.Close()

	invalidRepo := &gitalypb.Repository{StorageName: "fake", RelativePath: "path"}

	testCases := []struct {
		description string
		repo        *gitalypb.Repository
	}{
		{repo: invalidRepo, description: "Invalid repo"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {

			request := &gitalypb.RepositorySizeRequest{
				Repository: testCase.repo,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := client.RepositorySize(ctx, request)
			testhelper.RequireGrpcError(t, err, codes.InvalidArgument)
		})
	}
}

func TestSuccessfulGetObjectDirectorySizeRequest(t *testing.T) {
	server, serverSocketPath := runRepoServer(t)
	defer server.Stop()

	client, conn := newRepositoryClient(t, serverSocketPath)
	defer conn.Close()

	request := &gitalypb.GetObjectDirectorySizeRequest{
		Repository: testhelper.TestRepository(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	response, err := client.GetObjectDirectorySize(ctx, request)
	require.NoError(t, err)
	// We can't test for an exact size because it will be different for systems with different sector sizes,
	// so we settle for anything greater than zero.
	require.True(t, response.Size > 0, "size must be greater than zero")
}
