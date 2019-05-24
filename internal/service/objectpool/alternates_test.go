package objectpool

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitaly/internal/git"
	"gitlab.com/gitlab-org/gitaly/internal/git/objectpool"
	"gitlab.com/gitlab-org/gitaly/internal/testhelper"
)

func TestDisconnectGitAlternates(t *testing.T) {
	server, serverSocketPath := runObjectPoolServer(t)
	defer server.Stop()

	client, conn := newObjectPoolClient(t, serverSocketPath)
	defer conn.Close()

	ctx, cancel := testhelper.Context()
	defer cancel()

	testRepo, testRepoPath, cleanupFn := testhelper.NewTestRepo(t)
	defer cleanupFn()

	pool, err := objectpool.NewObjectPool(testRepo.GetStorageName(), testhelper.NewTestObjectPoolName(t))
	require.NoError(t, err)
	defer pool.Remove(ctx)

	require.NoError(t, pool.Create(ctx, testRepo))
	require.NoError(t, pool.Link(ctx, testRepo))
	testhelper.MustRunCommand(t, nil, "git", "-C", testRepoPath, "gc")

	existingObjectID := "55bc176024cfa3baaceb71db584c7e5df900ea65"

	// Corrupt the repository to check that existingObjectID can no longer be found
	altPath, err := git.InfoAlternatesPath(testRepo)
	require.NoError(t, err, "find info/alternates")
	require.NoError(t, os.RemoveAll(altPath))

	cmd, err := git.Command(ctx, testRepo, "cat-file", "-e", existingObjectID)
	require.NoError(t, err)
	require.Error(t, cmd.Wait(), "expect cat-file to fail because object cannot be found")

	require.NoError(t, pool.Link(ctx, testRepo))
	require.FileExists(t, altPath, "objects/info/alternates should be back")

	// At this point we know that the repository has access to
	// existingObjectID, but only if objects/info/alternates is in place.

	_, err = client.DisconnectGitAlternates(ctx, &gitalypb.DisconnectGitAlternatesRequest{Repository: testRepo})
	require.NoError(t, err, "call DisconnectGitAlternates")

	// Check that the object can still be found, even though
	// objects/info/alternates is gone. This is the purpose of
	// DisconnectGitAlternates.
	testhelper.AssertFileNotExists(t, altPath)
	testhelper.MustRunCommand(t, nil, "git", "-C", testRepoPath, "cat-file", "-e", existingObjectID)
}

func TestDisconnectGitAlternatesNoAlternates(t *testing.T) {
	server, serverSocketPath := runObjectPoolServer(t)
	defer server.Stop()

	client, conn := newObjectPoolClient(t, serverSocketPath)
	defer conn.Close()

	ctx, cancel := testhelper.Context()
	defer cancel()

	testRepo, testRepoPath, cleanupFn := testhelper.NewTestRepo(t)
	defer cleanupFn()

	altPath, err := git.InfoAlternatesPath(testRepo)
	require.NoError(t, err, "find info/alternates")
	testhelper.AssertFileNotExists(t, altPath)

	_, err = client.DisconnectGitAlternates(ctx, &gitalypb.DisconnectGitAlternatesRequest{Repository: testRepo})
	require.NoError(t, err, "call DisconnectGitAlternates on repository without alternates")

	testhelper.MustRunCommand(t, nil, "git", "-C", testRepoPath, "fsck")
}

func TestDisconnectGitAlternatesUnexpectedAlternates(t *testing.T) {
	server, serverSocketPath := runObjectPoolServer(t)
	defer server.Stop()

	client, conn := newObjectPoolClient(t, serverSocketPath)
	defer conn.Close()

	ctx, cancel := testhelper.Context()
	defer cancel()

	testCases := []struct {
		desc       string
		altContent string
	}{
		{desc: "multiple alternates", altContent: "/foo/bar\n/qux/baz\n"},
		{desc: "directory not found", altContent: "/does/not/exist/\n"},
		{desc: "not a directory", altContent: "../HEAD\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			testRepo, _, cleanupFn := testhelper.NewTestRepo(t)
			defer cleanupFn()

			altPath, err := git.InfoAlternatesPath(testRepo)
			require.NoError(t, err, "find info/alternates")

			require.NoError(t, ioutil.WriteFile(altPath, []byte(tc.altContent), 0644))

			_, err = client.DisconnectGitAlternates(ctx, &gitalypb.DisconnectGitAlternatesRequest{Repository: testRepo})
			require.Error(t, err, "call DisconnectGitAlternates on repository with unexpected objects/info/alternates")

			contentAfterRPC, err := ioutil.ReadFile(altPath)
			require.NoError(t, err, "read back objects/info/alternates")
			require.Equal(t, tc.altContent, string(contentAfterRPC), "objects/info/alternates content should not have changed")
		})
	}
}

func TestRemoveAlternatesIfOk(t *testing.T) {
	ctx, cancel := testhelper.Context()
	defer cancel()

	testRepo, testRepoPath, cleanupFn := testhelper.NewTestRepo(t)
	defer cleanupFn()

	altPath, err := git.InfoAlternatesPath(testRepo)
	require.NoError(t, err, "find info/alternates")
	altContent := "/var/empty\n"
	require.NoError(t, ioutil.WriteFile(altPath, []byte(altContent), 0644), "write alternates file")

	// Intentionally break the repository, so that 'git fsck' will fail later.
	testhelper.MustRunCommand(t, nil, "sh", "-c", fmt.Sprintf("rm %s/objects/pack/*.pack", testRepoPath))

	altBackup := altPath + ".backup"

	err = removeAlternatesIfOk(ctx, testRepo, altPath, altBackup)
	require.Error(t, err, "removeAlternatesIfOk should fail")
	require.IsType(t, &fsckError{}, err, "error must be because of fsck")

	// We expect objects/info/alternates to have been restored when
	// removeAlternatesIfOk returned.
	assertAlternates(t, altPath, altContent)

	// We expect the backup alternates file to still exist.
	assertAlternates(t, altBackup, altContent)
}

func assertAlternates(t *testing.T, altPath string, altContent string) {
	actualContent, err := ioutil.ReadFile(altPath)
	require.NoError(t, err, "read %s after fsck failure", altPath)

	require.Equal(t, altContent, string(actualContent), "%s content after fsck failure", altPath)
}
