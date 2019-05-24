package objectpool

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	grpc_logrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	"gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitaly/internal/git"
	"gitlab.com/gitlab-org/gitaly/internal/helper"
)

// DisconnectGitAlternates is a slightly dangerous RPC. It optimistically
// hard-links all alternate objects we might need, and then temporarily
// removes (renames) objects/info/alternates and runs 'git fsck'. If we
// are unlucky that leaves the repository in a broken state during 'git
// fsck'. If we are very unlucky and Gitaly crashes, the repository stays
// in a broken state until an administrator intervenes and restores the
// backed-up copy of objects/info/alternates.
func (s *server) DisconnectGitAlternates(ctx context.Context, req *gitalypb.DisconnectGitAlternatesRequest) (*gitalypb.DisconnectGitAlternatesResponse, error) {
	repo := req.Repository

	if repo == nil {
		return nil, helper.ErrInvalidArgument(errors.New("no repository"))
	}

	if err := disconnectAlternates(ctx, repo); err != nil {
		return nil, helper.ErrInternal(err)
	}

	return &gitalypb.DisconnectGitAlternatesResponse{}, nil
}

func disconnectAlternates(ctx context.Context, repo *gitalypb.Repository) error {
	repoPath, err := helper.GetRepoPath(repo)
	if err != nil {
		return err
	}

	altFile, err := git.InfoAlternatesPath(repo)
	if err != nil {
		return err
	}

	altContents, err := ioutil.ReadFile(altFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	altDir := strings.TrimSpace(string(altContents))
	if strings.Contains(altDir, "\n") {
		return &invalidAlternatesError{altContents: altContents}
	}

	if !filepath.IsAbs(altDir) {
		altDir = filepath.Join(repoPath, "objects", altDir)
	}

	stat, err := os.Stat(altDir)
	if err != nil {
		return err
	}

	if !stat.IsDir() {
		return &invalidAlternatesError{altContents: altContents}
	}

	objectFiles, err := findObjectFiles(altDir)
	if err != nil {
		return err
	}

	for _, path := range objectFiles {
		source := filepath.Join(altDir, path)
		target := filepath.Join(repoPath, "objects", path)

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		if err := os.Link(source, target); err != nil {
			if os.IsExist(err) {
				continue
			}

			return err
		}
	}

	backupFile, err := newBackupFile(altFile)
	if err != nil {
		return err
	}

	return removeAlternatesIfOk(ctx, repo, altFile, backupFile)
}

func newBackupFile(altFile string) (string, error) {
	randSuffix := make([]byte, 6)
	if _, err := io.ReadFull(rand.Reader, randSuffix); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s.%d.%x", altFile, time.Now().Unix(), randSuffix), nil
}

func findObjectFiles(altDir string) ([]string, error) {
	var objectFiles []string
	if walkErr := filepath.Walk(altDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(altDir, path)
		if err != nil {
			return err
		}

		if strings.HasPrefix(rel, "info/") {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		objectFiles = append(objectFiles, rel)

		return nil
	}); walkErr != nil {
		return nil, walkErr
	}

	sort.Sort(objectPaths(objectFiles))

	return objectFiles, nil
}

type fsckError struct{ error }

func (fe *fsckError) Error() string {
	return fmt.Sprintf("git fsck error while disconnected: %v", fe.error)
}

type invalidAlternatesError struct {
	altContents []byte
}

func (e *invalidAlternatesError) Error() string {
	return fmt.Sprintf("invalid content in objects/info/alternates: %q", e.altContents)
}

// removeAlternatesIfOk is dangerous. We optimistically temporarily
// rename objects/info/alternates, and run `git fsck` to see if the
// resulting repo is connected. If this fails we restore
// objects/info/alternates. If the repo is not connected for whatever
// reason, then until this function returns, probably **all concurrent
// RPC calls to the repo will fail**. Also, if Gitaly crashes in the
// middle of this function, the repo is left in a broken state. We do
// take care to leave a copy of the alternates file, so that it can be
// manually restored by an administrator if needed.
func removeAlternatesIfOk(ctx context.Context, repo *gitalypb.Repository, altFile, backupFile string) error {
	if err := os.Rename(altFile, backupFile); err != nil {
		return err
	}

	rollback := true
	defer func() {
		if !rollback {
			return
		}

		logger := grpc_logrus.Extract(ctx)

		// If we would do a os.Rename, and then someone else comes and clobbers
		// our file, it's gone forever. This trick with os.Link and os.Rename
		// is equivalent to "cp $backupFile $altFile", meaning backupFile is
		// preserved for possible forensic use.
		tmp := backupFile + ".2"

		if err := os.Link(backupFile, tmp); err != nil {
			logger.WithError(err).Error("copy backup alternates file")
			return
		}

		if err := os.Rename(tmp, altFile); err != nil {
			logger.WithError(err).Error("restore backup alternates file")
		}
	}()

	cmd, err := git.Command(ctx, repo, "fsck", "--connectivity-only")
	if err != nil {
		return err
	}

	if err := cmd.Wait(); err != nil {
		return &fsckError{error: err}
	}

	rollback = false
	return nil
}

type objectPaths []string

func (o objectPaths) Len() int      { return len(o) }
func (o objectPaths) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o objectPaths) Less(i, j int) bool {
	return objectPriority(o[i]) <= objectPriority(o[j])
}

// Based on pack_copy_priority in git/tmp-objdir.c
func objectPriority(name string) int {
	if !strings.HasPrefix(name, "pack") {
		return 0
	}
	if strings.HasSuffix(name, ".keep") {
		return 1
	}
	if strings.HasSuffix(name, ".pack") {
		return 2
	}
	if strings.HasSuffix(name, ".idx") {
		return 3
	}
	return 4
}
