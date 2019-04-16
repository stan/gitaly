package repository

import (
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitaly/internal/testhelper"
	"google.golang.org/grpc/codes"
)

func TestGetRawChanges(t *testing.T) {
	server, serverSocketPath := runRepoServer(t)
	defer server.Stop()

	client, conn := newRepositoryClient(t, serverSocketPath)
	defer conn.Close()
	testRepo, _, cleanupFn := testhelper.NewTestRepo(t)
	defer cleanupFn()

	testCases := []struct {
		oldRev  string
		newRev  string
		changes []*gitalypb.GetRawChangesResponse_RawChange
	}{
		{
			oldRev: "55bc176024cfa3baaceb71db584c7e5df900ea65",
			newRev: "7975be0116940bf2ad4321f79d02a55c5f7779aa",
			changes: []*gitalypb.GetRawChangesResponse_RawChange{
				{
					BlobId:    "c60514b6d3d6bf4bec1030f70026e34dfbd69ad5",
					Size:      824,
					NewPath:   "README.md",
					OldPath:   "README.md",
					Operation: gitalypb.GetRawChangesResponse_RawChange_MODIFIED,
					OldMode:   0100644,
					NewMode:   0100644,
				},
				{
					BlobId:    "723c2c3f4c8a2a1e957f878c8813acfc08cda2b6",
					Size:      1219696,
					NewPath:   "files/images/emoji.png",
					Operation: gitalypb.GetRawChangesResponse_RawChange_ADDED,
					NewMode:   0100644,
				},
			},
		},
		{
			oldRev: "0000000000000000000000000000000000000000",
			newRev: "1a0b36b3cdad1d2ee32457c102a8c0b7056fa863",
			changes: []*gitalypb.GetRawChangesResponse_RawChange{
				{
					BlobId:    "470ad2fcf1e33798f1afc5781d08e60c40f51e7a",
					Size:      231,
					NewPath:   ".gitignore",
					Operation: gitalypb.GetRawChangesResponse_RawChange_ADDED,
					NewMode:   0100644,
				},
				{
					BlobId:    "50b27c6518be44c42c4d87966ae2481ce895624c",
					Size:      1075,
					NewPath:   "LICENSE",
					Operation: gitalypb.GetRawChangesResponse_RawChange_ADDED,
					NewMode:   0100644,
				},
				{
					BlobId:    "faaf198af3a36dbf41961466703cc1d47c61d051",
					Size:      55,
					NewPath:   "README.md",
					Operation: gitalypb.GetRawChangesResponse_RawChange_ADDED,
					NewMode:   0100644,
				},
			},
		},
		{
			oldRev: "6b8dc4a827797aa025ff6b8f425e583858a10d4f",
			newRev: "06041ab2037429d243a38abb55957818dd9f948d",
			changes: []*gitalypb.GetRawChangesResponse_RawChange{
				{
					BlobId:    "c84acd1ff0b844201312052f9bb3b7259eb2e177",
					Size:      23,
					NewPath:   "files/executables/ls",
					OldPath:   "files/executables/ls",
					Operation: gitalypb.GetRawChangesResponse_RawChange_MODIFIED,
					OldMode:   0100755,
					NewMode:   0100644,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("old:%s,new:%s", tc.oldRev, tc.newRev), func(t *testing.T) {
			ctx, cancel := testhelper.Context()
			defer cancel()

			req := &gitalypb.GetRawChangesRequest{
				Repository:   testRepo,
				FromRevision: tc.oldRev,
				ToRevision:   tc.newRev,
			}

			stream, err := client.GetRawChanges(ctx, req)
			require.NoError(t, err)

			changes := collectChanges(t, stream)
			require.Equal(t, tc.changes, changes)
		})
	}
}

func TestGetRawChangesSpecialCharacters(t *testing.T) {
	// We know that 'git diff --raw' sometimes quotes "special characters" in
	// paths, and that this can result in incorrect results from the
	// GetRawChanges RPC, see
	// https://gitlab.com/gitlab-org/gitaly/issues/1444. The definition of
	// "special" is under core.quotePath in `git help config`.
	//
	// This test looks for a specific path known to contain special
	// characters.

	server, serverSocketPath := runRepoServer(t)
	defer server.Stop()

	client, conn := newRepositoryClient(t, serverSocketPath)
	defer conn.Close()
	testRepo, _, cleanupFn := testhelper.NewTestRepo(t)
	defer cleanupFn()

	ctx, cancel := testhelper.Context()
	defer cancel()

	req := &gitalypb.GetRawChangesRequest{
		Repository:   testRepo,
		FromRevision: "cfe32cf61b73a0d5e9f13e774abde7ff789b1660",
		ToRevision:   "913c66a37b4a45b9769037c55c2d238bd0942d2e",
	}

	stream, err := client.GetRawChanges(ctx, req)
	require.NoError(t, err)

	changes := collectChanges(t, stream)

	nChangedFiles := 23
	require.Len(t, changes, nChangedFiles)

	specialFileIdx := 11
	require.Equal(t, "encoding/テスト.txt", changes[specialFileIdx].NewPath)
}

func collectChanges(t *testing.T, stream gitalypb.RepositoryService_GetRawChangesClient) []*gitalypb.GetRawChangesResponse_RawChange {
	var changes []*gitalypb.GetRawChangesResponse_RawChange
	var err error

	for err == nil {
		var msg *gitalypb.GetRawChangesResponse
		msg, err = stream.Recv()
		changes = append(changes, msg.GetRawChanges()...)
	}
	require.Equal(t, io.EOF, err)

	return changes
}

func TestGetRawChangesFailures(t *testing.T) {
	server, serverSocketPath := runRepoServer(t)
	defer server.Stop()

	client, conn := newRepositoryClient(t, serverSocketPath)
	defer conn.Close()

	testRepo, _, cleanupFn := testhelper.NewTestRepo(t)
	defer cleanupFn()

	testCases := []struct {
		oldRev         string
		newRev         string
		code           codes.Code
		omitRepository bool
	}{
		{
			oldRev: "",
			newRev: "1a0b36b3cdad1d2ee32457c102a8c0b7056fa863",
			code:   codes.InvalidArgument,
		},
		{
			oldRev:         "cfe32cf61b73a0d5e9f13e774abde7ff789b1660",
			newRev:         "913c66a37b4a45b9769037c55c2d238bd0942d2e",
			code:           codes.InvalidArgument,
			omitRepository: true,
		},
		{
			// A Gitaly commit, unresolvable in gitlab-test
			oldRev: "32800ed8206c0087f65e90a1a396b76d3c33f648",
			newRev: "1a0b36b3cdad1d2ee32457c102a8c0b7056fa863",
			code:   codes.InvalidArgument,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("old:%s,new:%s", tc.oldRev, tc.newRev), func(t *testing.T) {
			ctx, cancel := testhelper.Context()
			defer cancel()

			req := &gitalypb.GetRawChangesRequest{
				Repository:   testRepo,
				FromRevision: tc.oldRev,
				ToRevision:   tc.newRev,
			}

			if tc.omitRepository {
				req.Repository = nil
			}

			resp, err := client.GetRawChanges(ctx, req)
			require.NoError(t, err)

			for err == nil {
				_, err = resp.Recv()
			}

			testhelper.RequireGrpcError(t, err, tc.code)
		})
	}
}

func TestGetRawChangesManyFiles(t *testing.T) {
	server, serverSocketPath := runRepoServer(t)
	defer server.Stop()

	client, conn := newRepositoryClient(t, serverSocketPath)
	defer conn.Close()

	testRepo, _, cleanupFn := testhelper.NewTestRepo(t)
	defer cleanupFn()

	ctx, cancel := testhelper.Context()
	defer cancel()

	initCommit := "1a0b36b3cdad1d2ee32457c102a8c0b7056fa863"
	req := &gitalypb.GetRawChangesRequest{
		Repository:   testRepo,
		FromRevision: initCommit,
		ToRevision:   "many_files",
	}

	c, err := client.GetRawChanges(ctx, req)
	require.NoError(t, err)

	changes := collectChanges(t, c)

	require.True(t, len(changes) >= 1041, "Changes has to contain at least 1041 changes")
}

func TestGetRawChangesMappingOperations(t *testing.T) {
	server, serverSocketPath := runRepoServer(t)
	defer server.Stop()

	client, conn := newRepositoryClient(t, serverSocketPath)
	defer conn.Close()

	testRepo, _, cleanupFn := testhelper.NewTestRepo(t)
	defer cleanupFn()

	ctx, cancel := testhelper.Context()
	defer cancel()

	req := &gitalypb.GetRawChangesRequest{
		Repository:   testRepo,
		FromRevision: "1b12f15a11fc6e62177bef08f47bc7b5ce50b141",
		ToRevision:   "94bb47ca1297b7b3731ff2a36923640991e9236f",
	}

	c, err := client.GetRawChanges(ctx, req)
	require.NoError(t, err)
	msg, err := c.Recv()
	require.NoError(t, err)

	ops := []gitalypb.GetRawChangesResponse_RawChange_Operation{}
	for _, change := range msg.GetRawChanges() {
		ops = append(ops, change.GetOperation())
	}

	expected := []gitalypb.GetRawChangesResponse_RawChange_Operation{
		gitalypb.GetRawChangesResponse_RawChange_RENAMED,
		gitalypb.GetRawChangesResponse_RawChange_MODIFIED,
		gitalypb.GetRawChangesResponse_RawChange_ADDED,
	}

	firstChange := &gitalypb.GetRawChangesResponse_RawChange{
		BlobId:    "53855584db773c3df5b5f61f72974cb298822fbb",
		Size:      22846,
		NewPath:   "CHANGELOG.md",
		OldPath:   "CHANGELOG",
		Operation: gitalypb.GetRawChangesResponse_RawChange_RENAMED,
		OldMode:   0100644,
		NewMode:   0100644,
	}

	require.Equal(t, firstChange, msg.GetRawChanges()[0])
	require.EqualValues(t, expected, ops)
}
