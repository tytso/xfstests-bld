/*
Package git implements multiple versions of git repositories.

Repository is used for kernel compilation and git bisect.
RemoteRepository is used for git repo watcher.
*/
package git

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"

	"gce-server/util/check"
	"gce-server/util/server"
)

// configurable constants for git utility functions
const (
	RepoRootDir       = "/cache/repositories/"
	RefRepoDir        = RepoRootDir + "linux.reference"
	RefRepoURL        = "https://git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git"
	BuildUploadScript = "/usr/local/lib/gce-build-upload-kernel"
	watchInterval     = 10
)

// Repository represents a git repo
// Uses a lock to avoid concurrent access
type Repository struct {
	id   string
	url  string
	base string
	dir  string
	lock sync.Mutex
}

// RemoteRepository represents a remote repo
// No need for locks since git watcher is protected by channels
type RemoteRepository struct {
	url    string
	branch string
	head   string
}

func init() {
	err := check.CreateDir(RepoRootDir)
	if err != nil {
		panic(err)
	}
}

/*
NewRepository clones a repository with reference to a base repo.

Each repoURL binds to a default directory which should be used for creating new copies
of the same repo (used in bisect). If id matches the default repo id, return that repo.
It does not overwrite the directory if it already exists.
*/
func NewRepository(id string, repoURL string, writer io.Writer) (*Repository, error) {
	if id == "" {
		return nil, fmt.Errorf("repo id not specified")
	}
	if !check.DirExists(RefRepoDir) {
		cmd := exec.Command("git", "clone", "--mirror", RefRepoURL, RefRepoDir)
		err := check.Run(cmd, check.RootDir, check.EmptyEnv, writer, writer)
		if err != nil {
			return nil, err
		}
	}

	base, err := ParseURL(repoURL)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse repo url")
	}

	baseRepoDir := RepoRootDir + base + "/"
	if !check.DirExists(baseRepoDir) {
		cmd := exec.Command("git", "clone", "--reference", RefRepoDir, repoURL, baseRepoDir)
		err = check.Run(cmd, check.RootDir, check.EmptyEnv, writer, writer)
		if err != nil {
			os.RemoveAll(baseRepoDir)
			return nil, err
		}
	}

	repo := Repository{
		id:   id,
		url:  repoURL,
		base: base,
		dir:  RepoRootDir + id + "/",
	}

	if check.DirExists(repo.dir) {
		return &repo, nil
	}

	cmd := exec.Command("git", "clone", "--shared", baseRepoDir, repo.dir)
	err = check.Run(cmd, check.RootDir, check.EmptyEnv, writer, writer)
	if err != nil {
		os.RemoveAll(repo.dir)
		return nil, err
	}
	cmd = exec.Command("git", "remote", "set-url", "origin", repoURL)
	err = check.Run(cmd, repo.dir, check.EmptyEnv, writer, writer)
	if err != nil {
		os.RemoveAll(repo.dir)
		return nil, err
	}
	cmd = exec.Command("git", "fetch", "-q", "--all")
	err = check.Run(cmd, repo.dir, check.EmptyEnv, writer, writer)
	if err != nil {
		os.RemoveAll(repo.dir)
		return nil, err
	}

	return &repo, nil
}

// GetCommit returns the commit hash for current repo HEAD
func (repo *Repository) GetCommit(writer io.Writer) (string, error) {
	if !check.DirExists(repo.dir) {
		return "", fmt.Errorf("directory %s does not exist", repo.dir)
	}

	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := check.Output(cmd, repo.dir, check.EmptyEnv, writer)
	if err != nil {
		writer.Write([]byte(output))
		return "", err
	}

	return output[:len(output)-1], nil
}

// Checkout fetches from upstream and checkout to a commit hash.
func (repo *Repository) Checkout(commit string, writer io.Writer) error {
	repo.lock.Lock()
	defer repo.lock.Unlock()
	if !check.DirExists(repo.dir) {
		return fmt.Errorf("directory %s does not exist", repo.dir)
	}

	cmd := exec.Command("git", "fetch", "-q", "--all")
	err := check.Run(cmd, repo.dir, check.EmptyEnv, writer, writer)
	if err != nil {
		return err
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
	repo.lock.Lock()
	defer repo.lock.Unlock()
	if !check.DirExists(repo.dir) {
		return false, fmt.Errorf("directory %s does not exist", repo.dir)
	}

	cmd := exec.Command("git", "log", "--oneline", "-1", revision)
	err := check.Run(cmd, repo.dir, check.EmptyEnv, writer, writer)
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
	repo.lock.Lock()
	defer repo.lock.Unlock()
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
	err := check.Run(cmd, repo.dir, check.EmptyEnv, writer, writer)
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
	repo.lock.Lock()
	defer repo.lock.Unlock()
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
	repo.lock.Lock()
	defer repo.lock.Unlock()
	if !check.DirExists(repo.dir) {
		return fmt.Errorf("directory %s does not exist", repo.dir)
	}

	cmd := exec.Command("git", "bisect", "reset")
	return check.Run(cmd, repo.dir, check.EmptyEnv, writer, writer)
}

// BuildUpload builds the current kernel code and uploads image to GS.
// Script output is written into a given writer
func (repo *Repository) BuildUpload(gsBucket string, gsPath string, writer io.Writer) error {
	repo.lock.Lock()
	defer repo.lock.Unlock()
	if !check.DirExists(repo.dir) {
		return fmt.Errorf("directory %s does not exist", repo.dir)
	}

	cmd := exec.Command(BuildUploadScript)

	env := map[string]string{
		"GS_BUCKET": gsBucket,
		"GS_PATH":   gsPath,
	}

	err := check.Run(cmd, repo.dir, env, writer, writer)

	return err
}

// Delete removes repo from local storage
func (repo *Repository) Delete() error {
	repo.lock.Lock()
	defer repo.lock.Unlock()
	err := os.RemoveAll(repo.dir)
	return err
}

// Dir returns the repo directory
func (repo *Repository) Dir() string {
	return repo.dir
}

// NewRemoteRepository initiates a remote repo and get HEAD on given branch
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

// Update gets new HEAD and returns true if it has changed since last update
func (repo *RemoteRepository) Update() (bool, error) {
	head, err := getHead(repo.url, repo.branch)
	if err != nil {
		return false, err
	}
	if head != repo.head {
		repo.head = head
		return true, nil
	}

	return false, nil
}

// Head returns the current head
func (repo *RemoteRepository) Head() string {
	return repo.head
}

// getHead retrives the commit hash of the HEAD on a branch
func getHead(repoURL string, branch string) (string, error) {
	cmd := exec.Command("git", "ls-remote", "--heads", "--quiet", "--exit-code", repoURL, branch)
	output, err := check.Output(cmd, check.RootDir, check.EmptyEnv, os.Stderr)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 2 {
				return "", fmt.Errorf("branch is not found")
			}
		}
		return "", err
	}

	commit := strings.Fields(output)[0]
	return commit, nil
}

// ParseURL transforms a git url into a human readable directory string
// Format is hostname - last two parts of path - last 4 byte of md5 sum
// Clone with ssh key is not supported
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
