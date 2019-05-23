package testhelper

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_logrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitaly/internal/command"
	"gitlab.com/gitlab-org/gitaly/internal/config"
	"gitlab.com/gitlab-org/gitaly/internal/helper/fieldextractors"
	"gitlab.com/gitlab-org/gitaly/internal/helper/text"
	gitalylog "gitlab.com/gitlab-org/gitaly/internal/log"
	"gitlab.com/gitlab-org/gitaly/internal/storage"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TestRelativePath is the path inside its storage of the gitlab-test repo
const (
	TestRelativePath    = "gitlab-test.git"
	RepositoryAuthToken = "the-secret-token"
	DefaultStorageName  = "default"
	testGitEnv          = "testdata/git-env"
)

func init() {
	config.Config.Storages = []config.Storage{
		{Name: "default", Path: GitlabTestStoragePath()},
	}

	gitalylog.GrpcGo.SetLevel(log.WarnLevel)
	grpc_logrus.ReplaceGrpcLogger(log.NewEntry(gitalylog.GrpcGo))
}

// MustReadFile returns the content of a file or fails at once.
func MustReadFile(t *testing.T, filename string) []byte {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	return content
}

// GitlabTestStoragePath returns the storage path to the gitlab-test repo.
func GitlabTestStoragePath() string {
	// If TEST_REPO_STORAGE_PATH has been set (by the Makefile) then use that
	testRepoPath := os.Getenv("TEST_REPO_STORAGE_PATH")
	if testRepoPath != "" {
		testRepoPathAbs, err := filepath.Abs(testRepoPath)
		if err != nil {
			log.Fatal(err)
		}

		return testRepoPathAbs
	}

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("Could not get caller info")
	}
	return path.Join(path.Dir(currentFile), "testdata/data")
}

// GitalyServersMetadata returns a metadata pair for gitaly-servers to be used in
// inter-gitaly operations.
func GitalyServersMetadata(t *testing.T, serverSocketPath string) metadata.MD {
	gitalyServers := storage.GitalyServers{
		"default": {
			"address": serverSocketPath,
			"token":   RepositoryAuthToken,
		},
	}

	gitalyServersJSON, err := json.Marshal(gitalyServers)
	if err != nil {
		t.Fatal(err)
	}

	return metadata.Pairs("gitaly-servers", base64.StdEncoding.EncodeToString(gitalyServersJSON))
}

func testRepoValid(repo *gitalypb.Repository) bool {
	storagePath, _ := config.StoragePath(repo.GetStorageName())
	if _, err := os.Stat(path.Join(storagePath, repo.RelativePath, "objects")); err != nil {
		return false
	}

	return true
}

// TestRepository returns the `Repository` object for the gitlab-test repo.
// Tests should be calling this function instead of cloning the repo themselves.
// Tests that involve modifications to the repo should copy/clone the repo
// via the `Repository` returned from this function.
func TestRepository() *gitalypb.Repository {
	repo := &gitalypb.Repository{
		StorageName:        "default",
		RelativePath:       TestRelativePath,
		GlRepository:       "project-1",
		GitObjectDirectory: "objects/",
	}

	if !testRepoValid(repo) {
		panic("Test repo not found, did you run `make prepare-tests`?")
	}

	return repo
}

// RequireGrpcError asserts the passed err is of the same code as expectedCode.
func RequireGrpcError(t *testing.T, err error, expectedCode codes.Code) {
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}

	// Check that the code matches
	status, _ := status.FromError(err)
	if code := status.Code(); code != expectedCode {
		t.Fatalf("Expected an error with code %v, got %v. The error was %q", expectedCode, code, err.Error())
	}
}

// MustRunCommand runs a command with an optional standard input and returns the standard output, or fails.
func MustRunCommand(t testing.TB, stdin io.Reader, name string, args ...string) []byte {
	cmd := exec.Command(name, args...)

	if name == "git" {
		cmd.Env = append(command.GitEnv, cmd.Env...)
	}

	if stdin != nil {
		cmd.Stdin = stdin
	}

	output, err := cmd.Output()
	if err != nil {
		stderr := err.(*exec.ExitError).Stderr
		if t == nil {
			log.Print(name, args)
			log.Printf("%s", stderr)
			log.Fatal(err)
		} else {
			t.Log(name, args)
			t.Logf("%s", stderr)
			t.Fatal(err)
		}
	}

	return output
}

// authorSortofEqual tests if two `CommitAuthor`s have the same name and email.
//  useful when creating commits in the tests.
func authorSortofEqual(a, b *gitalypb.CommitAuthor) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	return bytes.Equal(a.GetName(), b.GetName()) &&
		bytes.Equal(a.GetEmail(), b.GetEmail())
}

// AuthorsEqual tests if two `CommitAuthor`s are equal
func AuthorsEqual(a *gitalypb.CommitAuthor, b *gitalypb.CommitAuthor) bool {
	return authorSortofEqual(a, b) &&
		a.GetDate().Seconds == b.GetDate().Seconds
}

// GitCommitEqual tests if two `GitCommit`s are equal
func GitCommitEqual(a, b *gitalypb.GitCommit) error {
	if !authorSortofEqual(a.GetAuthor(), b.GetAuthor()) {
		return fmt.Errorf("author does not match: %v != %v", a.GetAuthor(), b.GetAuthor())
	}
	if !authorSortofEqual(a.GetCommitter(), b.GetCommitter()) {
		return fmt.Errorf("commiter does not match: %v != %v", a.GetCommitter(), b.GetCommitter())
	}
	if !bytes.Equal(a.GetBody(), b.GetBody()) {
		return fmt.Errorf("body differs: %q != %q", a.GetBody(), b.GetBody())
	}
	if !bytes.Equal(a.GetSubject(), b.GetSubject()) {
		return fmt.Errorf("subject differs: %q != %q", a.GetSubject(), b.GetSubject())
	}
	if strings.Compare(a.GetId(), b.GetId()) != 0 {
		return fmt.Errorf("id does not match: %q != %q", a.GetId(), b.GetId())
	}
	if len(a.GetParentIds()) != len(b.GetParentIds()) {
		return fmt.Errorf("ParentId does not match: %v != %v", a.GetParentIds(), b.GetParentIds())
	}

	for i, pid := range a.GetParentIds() {
		pid2 := b.GetParentIds()[i]
		if strings.Compare(pid, pid2) != 0 {
			return fmt.Errorf("parent id mismatch: %v != %v", pid, pid2)
		}
	}

	return nil
}

// FindLocalBranchCommitAuthorsEqual tests if two `FindLocalBranchCommitAuthor`s are equal
func FindLocalBranchCommitAuthorsEqual(a *gitalypb.FindLocalBranchCommitAuthor, b *gitalypb.FindLocalBranchCommitAuthor) bool {
	return bytes.Equal(a.Name, b.Name) &&
		bytes.Equal(a.Email, b.Email) &&
		a.Date.Seconds == b.Date.Seconds
}

// FindLocalBranchResponsesEqual tests if two `FindLocalBranchResponse`s are equal
func FindLocalBranchResponsesEqual(a *gitalypb.FindLocalBranchResponse, b *gitalypb.FindLocalBranchResponse) bool {
	return a.CommitId == b.CommitId &&
		bytes.Equal(a.CommitSubject, b.CommitSubject) &&
		FindLocalBranchCommitAuthorsEqual(a.CommitAuthor, b.CommitAuthor) &&
		FindLocalBranchCommitAuthorsEqual(a.CommitCommitter, b.CommitCommitter)
}

// GetTemporaryGitalySocketFileName will return a unique, useable socket file name
func GetTemporaryGitalySocketFileName() string {
	tmpfile, err := ioutil.TempFile("", "gitaly.socket.")
	if err != nil {
		// No point in handling this error, panic
		panic(err)
	}

	name := tmpfile.Name()
	tmpfile.Close()
	os.Remove(name)

	return name
}

// ConfigureRuby configures Ruby settings for test purposes at run time.
func ConfigureRuby() {
	if dir := os.Getenv("GITALY_TEST_RUBY_DIR"); len(dir) > 0 {
		// Sometimes runtime.Caller is unreliable. This environment variable provides a bypass.
		config.Config.Ruby.Dir = dir
	} else {
		_, currentFile, _, ok := runtime.Caller(0)
		if !ok {
			log.Fatal("Could not get caller info")
		}
		config.Config.Ruby.Dir = path.Join(path.Dir(currentFile), "../../ruby")
	}

	if err := config.ConfigureRuby(); err != nil {
		log.Fatalf("validate ruby config: %v", err)
	}
}

// GetGitEnvData reads and returns the content of testGitEnv
func GetGitEnvData() (string, error) {
	gitEnvBytes, err := ioutil.ReadFile(testGitEnv)

	if err != nil {
		return "", err
	}

	return string(gitEnvBytes), nil
}

// NewTestGrpcServer creates a GRPC Server for testing purposes
func NewTestGrpcServer(tb testing.TB, streamInterceptors []grpc.StreamServerInterceptor, unaryInterceptors []grpc.UnaryServerInterceptor) *grpc.Server {
	logger := NewTestLogger(tb)
	logrusEntry := log.NewEntry(logger).WithField("test", tb.Name())

	ctxTagger := grpc_ctxtags.WithFieldExtractorForInitialReq(fieldextractors.FieldExtractor)
	ctxStreamTagger := grpc_ctxtags.StreamServerInterceptor(ctxTagger)
	ctxUnaryTagger := grpc_ctxtags.UnaryServerInterceptor(ctxTagger)

	streamInterceptors = append([]grpc.StreamServerInterceptor{ctxStreamTagger, grpc_logrus.StreamServerInterceptor(logrusEntry)}, streamInterceptors...)
	unaryInterceptors = append([]grpc.UnaryServerInterceptor{ctxUnaryTagger, grpc_logrus.UnaryServerInterceptor(logrusEntry)}, unaryInterceptors...)

	return grpc.NewServer(
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(streamInterceptors...)),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryInterceptors...)),
	)
}

// MustHaveNoChildProcess panics if it finds a running or finished child
// process. It waits for 2 seconds for processes to be cleaned up by other
// goroutines.
func MustHaveNoChildProcess() {
	waitDone := make(chan struct{})
	go func() {
		command.WaitAllDone()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
	}

	mustFindNoFinishedChildProcess()
	mustFindNoRunningChildProcess()
}

func mustFindNoFinishedChildProcess() {
	// Wait4(pid int, wstatus *WaitStatus, options int, rusage *Rusage) (wpid int, err error)
	//
	// We use pid -1 to wait for any child. We don't care about wstatus or
	// rusage. Use WNOHANG to return immediately if there is no child waiting
	// to be reaped.
	wpid, err := syscall.Wait4(-1, nil, syscall.WNOHANG, nil)
	if err == nil && wpid > 0 {
		panic(fmt.Errorf("wait4 found child process %d", wpid))
	}
}

func mustFindNoRunningChildProcess() {
	pgrep := exec.Command("pgrep", "-P", fmt.Sprintf("%d", os.Getpid()))
	desc := fmt.Sprintf("%q", strings.Join(pgrep.Args, " "))

	out, err := pgrep.Output()
	if err == nil {
		pidsComma := strings.Replace(text.ChompBytes(out), "\n", ",", -1)
		psOut, _ := exec.Command("ps", "-o", "pid,args", "-p", pidsComma).Output()
		panic(fmt.Errorf("found running child processes %s:\n%s", pidsComma, psOut))
	}

	if status, ok := command.ExitStatus(err); ok && status == 1 {
		// Exit status 1 means no processes were found
		return
	}

	panic(fmt.Errorf("%s: %v", desc, err))
}

// Context returns a cancellable context.
func Context() (context.Context, func()) {
	return context.WithCancel(context.Background())
}

// CreateRepo creates an temporary directory for a repo, without initializing it
func CreateRepo(t testing.TB, storagePath string) (repo *gitalypb.Repository, repoPath, relativePath string) {
	normalizedPrefix := strings.Replace(t.Name(), "/", "-", -1) //TempDir doesn't like a prefix containing slashes

	repoPath, err := ioutil.TempDir(storagePath, normalizedPrefix)
	require.NoError(t, err)

	relativePath, err = filepath.Rel(storagePath, repoPath)
	require.NoError(t, err)
	repo = &gitalypb.Repository{StorageName: "default", RelativePath: relativePath, GlRepository: "project-1"}

	return repo, repoPath, relativePath
}

// InitBareRepo creates a new bare repository
func InitBareRepo(t *testing.T) (*gitalypb.Repository, string, func()) {
	return initRepo(t, true)
}

// InitRepoWithWorktree creates a new repository with a worktree
func InitRepoWithWorktree(t *testing.T) (*gitalypb.Repository, string, func()) {
	return initRepo(t, false)
}

func initRepo(t *testing.T, bare bool) (*gitalypb.Repository, string, func()) {
	repo, repoPath, _ := CreateRepo(t, GitlabTestStoragePath())
	args := []string{"init"}
	if bare {
		args = append(args, "--bare")
	}

	MustRunCommand(t, nil, "git", append(args, repoPath)...)

	if !bare {
		repo.RelativePath = path.Join(repo.RelativePath, ".git")
	}

	return repo, repoPath, func() { os.RemoveAll(repoPath) }
}

// NewTestRepo creates a bare copy of the test repository.
func NewTestRepo(t testing.TB) (repo *gitalypb.Repository, repoPath string, cleanup func()) {
	return cloneTestRepo(t, true)
}

// NewTestRepoWithWorktree creates a copy of the test repository with a
// worktree. This is allows you to run normal 'non-bare' Git commands.
func NewTestRepoWithWorktree(t *testing.T) (repo *gitalypb.Repository, repoPath string, cleanup func()) {
	return cloneTestRepo(t, false)
}

func cloneTestRepo(t testing.TB, bare bool) (repo *gitalypb.Repository, repoPath string, cleanup func()) {
	storagePath := GitlabTestStoragePath()
	repo, repoPath, relativePath := CreateRepo(t, storagePath)
	testRepo := TestRepository()
	testRepoPath := path.Join(storagePath, testRepo.RelativePath)
	args := []string{"clone", "--no-hardlinks", "--dissociate"}

	if bare {
		args = append(args, "--bare")
	} else {
		// For non-bare repos the relative path is the .git folder inside the path
		repo.RelativePath = path.Join(relativePath, ".git")
	}

	MustRunCommand(t, nil, "git", append(args, testRepoPath, repoPath)...)

	return repo, repoPath, func() { os.RemoveAll(repoPath) }
}

// AddWorktreeArgs returns git command arguments for adding a worktree at the
// specified repo
func AddWorktreeArgs(repoPath, worktreeName string) []string {
	return []string{"-C", repoPath, "worktree", "add", "--detach", worktreeName}
}

// AddWorktree creates a worktree in the repository path for tests
func AddWorktree(t *testing.T, repoPath string, worktreeName string) {
	MustRunCommand(t, nil, "git", AddWorktreeArgs(repoPath, worktreeName)...)
}

// ConfigureGitalySSH configures the gitaly-ssh command for tests
func ConfigureGitalySSH() {
	var err error

	config.Config.BinDir, err = filepath.Abs("testdata/gitaly-libexec")
	if err != nil {
		log.Fatal(err)
	}

	goBuildArgs := []string{
		"build",
		"-o",
		path.Join(config.Config.BinDir, "gitaly-ssh"),
		"gitlab.com/gitlab-org/gitaly/cmd/gitaly-ssh",
	}
	MustRunCommand(nil, nil, "go", goBuildArgs...)
}

// GetRepositoryRefs gives a list of each repository ref as a string
func GetRepositoryRefs(t *testing.T, repoPath string) string {
	refs := MustRunCommand(t, nil, "git", "-C", repoPath, "for-each-ref")

	return string(refs)
}

// AssertFileNotExists asserts true if the file doesn't exist, false otherwise
func AssertFileNotExists(t *testing.T, path string) {
	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "file should not exist: %s", path)
}

// NewTestObjectPoolName returns a random pool repository name.
func NewTestObjectPoolName(t *testing.T) string {
	b := make([]byte, 5)
	_, err := io.ReadFull(rand.Reader, b)
	require.NoError(t, err)

	return fmt.Sprintf("pool-%x.git", b)
}

// CreateLooseRef creates a ref that points to master
func CreateLooseRef(t *testing.T, repoPath, refName string) {
	relRefPath := fmt.Sprintf("refs/heads/%s", refName)
	MustRunCommand(t, nil, "git", "-C", repoPath, "update-ref", relRefPath, "master")
	require.FileExists(t, filepath.Join(repoPath, relRefPath), "ref must be in loose file")
}
