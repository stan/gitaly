package rubyserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitaly/internal/config"
	"gitlab.com/gitlab-org/gitaly/internal/testhelper"
	"google.golang.org/grpc/codes"
)

func TestStopSafe(t *testing.T) {
	badServers := []*Server{
		nil,
		&Server{},
	}

	for _, bs := range badServers {
		bs.Stop()
	}
}

func TestSetHeaders(t *testing.T) {
	ctx, cancel := testhelper.Context()
	defer cancel()

	testCases := []struct {
		desc    string
		repo    *gitalypb.Repository
		errType codes.Code
		setter  func(context.Context, *gitalypb.Repository) (context.Context, error)
	}{
		{
			desc:    "SetHeaders invalid storage",
			repo:    &gitalypb.Repository{StorageName: "foo", RelativePath: "bar.git"},
			errType: codes.InvalidArgument,
			setter:  SetHeaders,
		},
		{
			desc:    "SetHeaders invalid rel path",
			repo:    &gitalypb.Repository{StorageName: testRepo.StorageName, RelativePath: "bar.git"},
			errType: codes.NotFound,
			setter:  SetHeaders,
		},
		{
			desc:    "SetHeaders OK",
			repo:    testRepo,
			errType: codes.OK,
			setter:  SetHeaders,
		},
		{
			desc:    "SetHeadersWithoutRepoCheck invalid storage",
			repo:    &gitalypb.Repository{StorageName: "foo", RelativePath: "bar.git"},
			errType: codes.InvalidArgument,
			setter:  SetHeadersWithoutRepoCheck,
		},
		{
			desc:    "SetHeadersWithoutRepoCheck invalid relative path",
			repo:    &gitalypb.Repository{StorageName: testRepo.StorageName, RelativePath: "bar.git"},
			errType: codes.OK,
			setter:  SetHeadersWithoutRepoCheck,
		},
		{
			desc:    "SetHeadersWithoutRepoCheck OK",
			repo:    testRepo,
			errType: codes.OK,
			setter:  SetHeadersWithoutRepoCheck,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			clientCtx, err := tc.setter(ctx, tc.repo)

			if tc.errType != codes.OK {
				testhelper.RequireGrpcError(t, err, tc.errType)
				assert.Nil(t, clientCtx)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, clientCtx)
			}
		})
	}
}

func TestPrepareSocketPath(t *testing.T) {
	// Without a config value set, default to `/tmp`
	prepareSocketPath()
	assert.True(t, strings.HasPrefix(socketDir, "/tmp"), "socketDir does not default to /tmp")

	pwd, err := os.Getwd()
	require.NoError(t, err)

	socketDir = ""
	config.Config.Ruby.SocketDir = pwd
	prepareSocketPath()
	assert.False(t, filepath.IsAbs(socketDir), fmt.Sprintf("socketDir: %v is absolute", socketDir))
}
