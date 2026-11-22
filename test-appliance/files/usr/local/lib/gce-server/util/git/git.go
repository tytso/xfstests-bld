/*
Package git implements multiple versions of git repositories:

Repository - local repo copies for kernel compilation and git bisect.
RemoteRepository - remote repos for git repo watcher.
*/
package git

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"thunk.org/gce-server/util/check"
	"thunk.org/gce-server/util/server"
)

// configurable constants for git utility functions
const (
	RepoRootDir        = "/cache/repositories/"
	RefRepoDir         = RepoRootDir + "linux.reference"
	RefRepoURL         = "https://kernel.googlesource.com/pub/scm/linux/kernel/git/stable/linux.git"
	RepoLockFileName   = ".repo-cleanup.lock"
	BuildUploadScript  = "/usr/local/lib/gce-build-upload-kernel"
	watchInterval      = 10
	remoteQueryTimeout = 2 * time.Minute
)

var (
	repoRootDir = RepoRootDir
	refRepoURL  = RefRepoURL

	repoRegistryMu sync.Mutex
	repoRegistry   = make(map[string]*Repository)
	pathLocks      = make(map[string]*sync.Mutex)
)

// Repository represents a local copy of git repo with locks to
// avoid concurrent access while permitting read-only git queries
// during kernel builds.
type Repository struct {
	id      string
	url     string
	base    string
	dir     string
	gitMu   sync.RWMutex
	buildMu sync.Mutex
}

// RemoteRepository represents a remote repo for queries.
type RemoteRepository struct {
	mu     sync.RWMutex
	url    string
	branch string
	head   string
}

func init() {
	err := check.CreateDir(RepoRootDir)
	if err != nil && !testing.Testing() {
		panic(err)
	}
}

func getPathLock(cleanPath string) *sync.Mutex {
	repoRegistryMu.Lock()
	defer repoRegistryMu.Unlock()
	mu, ok := pathLocks[cleanPath]
	if !ok {
		mu = &sync.Mutex{}
		pathLocks[cleanPath] = mu
	}
	return mu
}

func getOrCreateRepository(id, repoURL, base, dir string) *Repository {
	cleanDir := filepath.Clean(dir)
	repoRegistryMu.Lock()
	defer repoRegistryMu.Unlock()
	if repo, ok := repoRegistry[cleanDir]; ok {
		return repo
	}
	repo := &Repository{
		id:   id,
		url:  repoURL,
		base: base,
		dir:  dir,
	}
	repoRegistry[cleanDir] = repo
	return repo
}

func isReservedRepoID(id string) bool {
	return id == "linux.reference" || id == "bisects" || id == RepoLockFileName || strings.HasSuffix(id, ".tmp")
}

// acquireSharedRepoLock acquires an open-file-description shared flock on
// <repoRootDir>/.repo-cleanup.lock so concurrent KCS git/build operations
// proceed in parallel while blocking gce-repo-cleanup's exclusive flock.
func acquireSharedRepoLock() (func(), error) {
	if err := check.CreateDir(repoRootDir); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(repoRootDir, RepoLockFileName)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func ensureRefRepo(writer io.Writer) error {
	refDir := repoRootDir + "linux.reference"
	refClean := filepath.Clean(refDir)
	mu := getPathLock(refClean)
	mu.Lock()
	defer mu.Unlock()

	if check.DirExists(refDir) {
		return nil
	}
	if err := check.CreateDir(repoRootDir); err != nil {
		return err
	}
	tmpDir := refClean + ".tmp"
	_ = os.RemoveAll(tmpDir)
	cmd := exec.Command("git", "clone", "--mirror", refRepoURL, tmpDir)
	if err := check.Run(cmd, check.RootDir, check.EmptyEnv, writer, writer); err != nil {
		_ = os.RemoveAll(tmpDir)
		return err
	}
	if err := os.Rename(tmpDir, refClean); err != nil {
		_ = os.RemoveAll(tmpDir)
		return err
	}
	return nil
}

func ensureBaseRepo(base, repoURL string, writer io.Writer) (*Repository, error) {
	baseRepoDir := repoRootDir + base + "/"
	baseClean := filepath.Clean(baseRepoDir)
	mu := getPathLock(baseClean)
	mu.Lock()
	defer mu.Unlock()

	baseRepo := getOrCreateRepository(base, repoURL, base, baseRepoDir)
	if check.DirExists(baseRepoDir) {
		return baseRepo, nil
	}
	tmpDir := baseClean + ".tmp"
	_ = os.RemoveAll(tmpDir)
	refDir := repoRootDir + "linux.reference"
	cmd := exec.Command("git", "clone", "--reference", refDir, repoURL, tmpDir)
	if err := check.Run(cmd, check.RootDir, check.EmptyEnv, writer, writer); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}
	if err := os.Rename(tmpDir, baseClean); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}
	return baseRepo, nil
}

/*
NewRepository clones a repository with reference to a base repo.

Each repoURL binds to a default directory which should be used for creating new copies
of the same repo (used in bisect). If id matches the default repo id, return that repo.
It does not overwrite the directory if it already exists.
*/
func NewRepository(id string, repoURL string, writer io.Writer) (*Repository, error) {
	if err := check.ValidateTestID(id); err != nil {
		return nil, fmt.Errorf("invalid repo id: %w", err)
	}
	if isReservedRepoID(id) {
		return nil, fmt.Errorf("reserved repo id: %s", id)
	}
	return newRepositoryAt(id, repoURL, repoRootDir+id+"/", writer)
}

// NewBisectRepository clones an isolated per-bisect repository under the
// bisects/ subdirectory so user-supplied testIDs cannot alias linux.reference
// or shared base repositories.
func NewBisectRepository(id string, repoURL string, writer io.Writer) (*Repository, error) {
	if err := check.ValidateTestID(id); err != nil {
		return nil, fmt.Errorf("invalid bisect repo id: %w", err)
	}
	if isReservedRepoID(id) {
		return nil, fmt.Errorf("reserved bisect repo id: %s", id)
	}
	bisectsDir := repoRootDir + "bisects/"
	if err := check.CreateDir(bisectsDir); err != nil {
		return nil, err
	}
	return newRepositoryAt(id, repoURL, bisectsDir+id+"/", writer)
}

func newRepositoryAt(id string, repoURL string, targetDir string, writer io.Writer) (*Repository, error) {
	unlock, err := acquireSharedRepoLock()
	if err != nil {
		return nil, err
	}
	defer unlock()

	base, err := ParseURL(repoURL)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse repo url")
	}
	if err := check.ValidateTestID(base); err != nil {
		return nil, fmt.Errorf("invalid parsed repo dir: %w", err)
	}
	if isReservedRepoID(base) {
		return nil, fmt.Errorf("reserved parsed repo dir: %s", base)
	}

	if err := ensureRefRepo(writer); err != nil {
		return nil, err
	}
	baseRepo, err := ensureBaseRepo(base, repoURL, writer)
	if err != nil {
		return nil, err
	}

	targetClean := filepath.Clean(targetDir)
	if targetClean == filepath.Clean(baseRepo.dir) {
		return baseRepo, nil
	}

	targetMu := getPathLock(targetClean)
	targetMu.Lock()
	defer targetMu.Unlock()

	repo := getOrCreateRepository(id, repoURL, base, targetDir)
	repo.gitMu.Lock()
	defer repo.gitMu.Unlock()

	if check.DirExists(repo.dir) {
		return repo, nil
	}

	tmpDir := targetClean + ".tmp"
	_ = os.RemoveAll(tmpDir)

	baseRepo.gitMu.RLock()
	cmd := exec.Command("git", "clone", "--shared", baseRepo.dir, tmpDir)
	err = check.Run(cmd, check.RootDir, check.EmptyEnv, writer, writer)
	baseRepo.gitMu.RUnlock()
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}

	cmd = exec.Command("git", "remote", "set-url", "origin", repoURL)
	err = check.Run(cmd, tmpDir, check.EmptyEnv, writer, writer)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}
	cmd = exec.Command("git", "fetch", "-qpf", "--all")
	err = check.Run(cmd, tmpDir, check.EmptyEnv, writer, writer)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}
	if err := os.Rename(tmpDir, targetClean); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}

	return repo, nil
}

// GetCommit returns the commit hash for current repo HEAD.
func (repo *Repository) GetCommit(writer io.Writer) (string, error) {
	unlock, err := acquireSharedRepoLock()
	if err != nil {
		return "", err
	}
	defer unlock()

	repo.gitMu.RLock()
	defer repo.gitMu.RUnlock()
	if !check.DirExists(repo.dir) {
		return "", fmt.Errorf("directory %s does not exist", repo.dir)
	}

	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := check.Output(cmd, repo.dir, check.EmptyEnv, writer)
	if err != nil {
		writer.Write([]byte(output))
		return "", err
	}

	return strings.TrimRight(output, "\r\n"), nil
}

// Checkout fetches from upstream and checkout to a commit hash.
func (repo *Repository) Checkout(commit string, writer io.Writer) error {
	unlock, err := acquireSharedRepoLock()
	if err != nil {
		return err
	}
	defer unlock()

	repo.gitMu.Lock()
	defer repo.gitMu.Unlock()
	if !check.DirExists(repo.dir) {
		return fmt.Errorf("directory %s does not exist", repo.dir)
	}

	isHexCommit, _ := regexp.MatchString("^[0-9a-fA-F]+$", commit)
	if isHexCommit && len(commit) > 6 {
		cmd := exec.Command("git", "checkout", "-q", "-f", commit)
		err := check.Run(cmd, repo.dir, check.EmptyEnv, writer, writer)
		if err == nil {
			return nil
		}
	}
	cmd := exec.Command("git", "fetch", "-qpf", "--all")
	err = check.Run(cmd, repo.dir, check.EmptyEnv, writer, writer)
	if err != nil {
		return err
	}
	cmd = exec.Command("git", "checkout", "-q", "-f", "origin/"+commit)
	err = check.Run(cmd, repo.dir, check.EmptyEnv, writer, writer)
	if err == nil {
		return nil
	}
	cmd = exec.Command("git", "checkout", "-q", "-f", commit)
	err = check.Run(cmd, repo.dir, check.EmptyEnv, writer, writer)
	if err != nil {
		return err
	}

	return nil
}

// Valid checks the given revision and returns true if it's valid.
func (repo *Repository) Valid(revision string, writer io.Writer) (bool, error) {
	unlock, err := acquireSharedRepoLock()
	if err != nil {
		return false, err
	}
	defer unlock()

	repo.gitMu.RLock()
	defer repo.gitMu.RUnlock()
	if !check.DirExists(repo.dir) {
		return false, fmt.Errorf("directory %s does not exist", repo.dir)
	}

	cmd := exec.Command("git", "log", "--oneline", "-1", revision)
	err = check.Run(cmd, repo.dir, check.EmptyEnv, writer, writer)
	if err != nil {
		return false, err
	}
	return true, nil
}

/*
BisectStart starts a git bisect on a repository.

It uses badCommit and goodCommits to narrow down the search path.
Current head is used if badCommit is empty, and throws error if
goodCommits is empty. It returns true if git bisect has ended.

`git bisect start <bad> <good> [<good-2>...]` command fails silently
if <bad> is a branch, so we expand it explicitly.
*/
func (repo *Repository) BisectStart(badCommit string, goodCommits []string, writer io.Writer) (bool, error) {
	unlock, err := acquireSharedRepoLock()
	if err != nil {
		return false, err
	}
	defer unlock()

	repo.gitMu.Lock()
	defer repo.gitMu.Unlock()
	if len(goodCommits) == 0 {
		return false, fmt.Errorf("No good commits provided")
	}
	if !check.DirExists(repo.dir) {
		return false, fmt.Errorf("directory %s does not exist", repo.dir)
	}

	if badCommit == "" {
		badCommit = "HEAD"
	}

	cmd := exec.Command("git", "bisect", "start")
	err = check.Run(cmd, repo.dir, check.EmptyEnv, writer, writer)
	if err != nil {
		return false, err
	}

	cmd = exec.Command("git", "bisect", "bad", badCommit)
	err = check.Run(cmd, repo.dir, check.EmptyEnv, writer, writer)
	if err != nil {
		return false, err
	}

	args := []string{"bisect", "good"}
	args = append(args, goodCommits...)

	cmd = exec.Command("git", args...)
	output, err := check.Output(cmd, repo.dir, check.EmptyEnv, writer)
	if err != nil {
		writer.Write([]byte(output))
		return false, err
	}

	if strings.Contains(output, "is the first bad commit") {
		return true, nil
	}

	return false, nil
}

// BisectStep tells git bisect whether the current version is good or not
// and proceeds to the next step.
// It returns true if git bisect has ended.
func (repo *Repository) BisectStep(testResult server.ResultType, writer io.Writer) (bool, error) {
	unlock, err := acquireSharedRepoLock()
	if err != nil {
		return false, err
	}
	defer unlock()

	repo.gitMu.Lock()
	defer repo.gitMu.Unlock()
	if !check.DirExists(repo.dir) {
		return false, fmt.Errorf("directory %s does not exist", repo.dir)
	}
	var step string
	switch testResult {
	case server.Pass:
		step = "good"
	case server.Fail:
		fallthrough
	case server.Hang:
		fallthrough
	case server.Crash:
		step = "bad"
	case server.Error:
		step = "skip"
	default:
		return false, fmt.Errorf("unexpect test result value")
	}

	cmd := exec.Command("git", "bisect", step)
	output, err := check.Output(cmd, repo.dir, check.EmptyEnv, writer)
	if err != nil {
		writer.Write([]byte(output))
		return false, err
	}
	if strings.Contains(output, "is the first bad commit") {
		return true, nil
	}

	return false, nil
}

// BisectLog returns bisect log output.
func (repo *Repository) BisectLog(writer io.Writer) (string, error) {
	unlock, err := acquireSharedRepoLock()
	if err != nil {
		return "", err
	}
	defer unlock()

	repo.gitMu.RLock()
	defer repo.gitMu.RUnlock()
	if !check.DirExists(repo.dir) {
		return "", fmt.Errorf("directory %s does not exist", repo.dir)
	}

	cmd := exec.Command("git", "bisect", "log")
	output, err := check.Output(cmd, repo.dir, check.EmptyEnv, writer)
	if err != nil {
		writer.Write([]byte(output))
		return "", err
	}

	return output, nil
}

// BisectReset resets the current git bisect.
func (repo *Repository) BisectReset(writer io.Writer) error {
	unlock, err := acquireSharedRepoLock()
	if err != nil {
		return err
	}
	defer unlock()

	repo.gitMu.Lock()
	defer repo.gitMu.Unlock()
	if !check.DirExists(repo.dir) {
		return fmt.Errorf("directory %s does not exist", repo.dir)
	}

	cmd := exec.Command("git", "bisect", "reset")
	return check.Run(cmd, repo.dir, check.EmptyEnv, writer, writer)
}

// BuildUpload builds the current kernel code and uploads image to GS.
// Script output is written into a given writer.
func (repo *Repository) BuildUpload(gsBucket string, gsPath string, gsConfig string, arch string, kConfigOpts string, kbuildOpts string, writer io.Writer) error {
	unlock, err := acquireSharedRepoLock()
	if err != nil {
		return err
	}
	defer unlock()

	repo.buildMu.Lock()
	defer repo.buildMu.Unlock()
	repo.gitMu.RLock()
	defer repo.gitMu.RUnlock()
	if !check.DirExists(repo.dir) {
		return fmt.Errorf("directory %s does not exist", repo.dir)
	}

	cmd := exec.Command(BuildUploadScript)

	env := map[string]string{
		"GS_BUCKET":    gsBucket,
		"GS_PATH":      gsPath,
		"GS_CONFIG":    gsConfig,
		"KCONFIG_OPTS": kConfigOpts,
		"KBUILD_OPTS":  kbuildOpts,
		"KBUILD_ARCH":  arch,
	}

	err = check.Run(cmd, repo.dir, env, writer, writer)

	return err
}

// Delete removes repo from local storage.
func (repo *Repository) Delete() error {
	unlock, err := acquireSharedRepoLock()
	if err != nil {
		return err
	}
	defer unlock()

	repo.buildMu.Lock()
	defer repo.buildMu.Unlock()
	repo.gitMu.Lock()
	defer repo.gitMu.Unlock()
	err = os.RemoveAll(repo.dir)
	repoRegistryMu.Lock()
	delete(repoRegistry, filepath.Clean(repo.dir))
	repoRegistryMu.Unlock()
	return err
}

// Dir returns the repo directory.
func (repo *Repository) Dir() string {
	return repo.dir
}

// NewRemoteRepository initiates a remote repo and get HEAD on given branch.
func NewRemoteRepository(repoURL string, branch string) (*RemoteRepository, error) {
	repo := RemoteRepository{
		url:    repoURL,
		branch: branch,
	}

	head, err := getHead(repo.url, repo.branch)
	if err != nil {
		return nil, err
	}
	repo.head = head

	return &repo, nil
}

// Update gets new HEAD and returns true if it has changed since last update.
func (repo *RemoteRepository) Update() (bool, error) {
	head, err := getHead(repo.url, repo.branch)
	if err != nil {
		return false, err
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if head != repo.head {
		repo.head = head
		return true, nil
	}

	return false, nil
}

// Head returns the current head.
func (repo *RemoteRepository) Head() string {
	repo.mu.RLock()
	defer repo.mu.RUnlock()
	return repo.head
}

// getHead retrives the commit hash of the HEAD on a branch.
func getHead(repoURL string, branch string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), remoteQueryTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--heads", "--quiet", "--exit-code", repoURL, branch)
	env := map[string]string{
		"GIT_TERMINAL_PROMPT": "0",
	}
	output, err := check.Output(cmd, check.RootDir, env, os.Stderr)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("git ls-remote timed out: %w", ctx.Err())
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 2 {
				return "", fmt.Errorf("branch is not found")
			}
		}
		return "", err
	}

	fields := strings.Fields(output)
	if len(fields) == 0 {
		return "", fmt.Errorf("empty ls-remote output")
	}
	return fields[0], nil
}

// ParseURL transforms a git url into a human readable directory string.
// Format: hostname - last two parts of path - last 4 byte of md5 sum
// Clone with ssh key is not supported.
func ParseURL(repoURL string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", err
	}
	paths := strings.Split(u.Path, "/")
	hash := md5.Sum([]byte(repoURL))

	name := []string{u.Hostname()}
	name = append(name, paths[len(paths)-2:]...)
	name = append(name, hex.EncodeToString(hash[len(hash)-4:]))

	return strings.Join(name, "-"), nil
}
