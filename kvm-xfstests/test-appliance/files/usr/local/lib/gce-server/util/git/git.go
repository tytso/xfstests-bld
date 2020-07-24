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

	"gce-server/util/check"
	"gce-server/util/server"

	"github.com/google/uuid"
)

// configurable constants for git utility functions
const (
	RepoRootDir       = "/root/repositories/"
	RefRepoDir        = RepoRootDir + "linux.reference"
	RefRepoURL        = "https://git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git"
	BuildUploadScript = "/usr/local/lib/gce-build-upload-kernel"
	watchInterval     = 10
)

// Repository represents a git repo
type Repository struct {
	id  string
	url string
}

// RemoteRepository represents a remote repo
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

// NewRepository clones a repository with reference to a base repo.
// The repo directory is named as id under RepoRootDir.
// It assumes each directory binds to a unique repo, and does not
// overwrite that directory if it already exists.
func NewRepository(id string, repoURL string) (*Repository, error) {
	if id == "" {
		return nil, fmt.Errorf("repo id not specified")
	}
	if !check.DirExists(RefRepoDir) {
		cmd := exec.Command("git", "clone", "--mirror", RefRepoURL, RefRepoDir)
		err := check.Run(cmd, check.RootDir, check.EmptyEnv, os.Stdout, os.Stderr)
		if err != nil {
			return nil, err
		}
	}

	repo := Repository{
		id:  id,
		url: repoURL,
	}

	repoDir := repo.Dir()
	if check.DirExists(repoDir) {
		return &repo, nil
	}

	cmd := exec.Command("git", "clone", "--reference", RefRepoDir, repoURL, repoDir)
	err := check.Run(cmd, check.RootDir, check.EmptyEnv, os.Stdout, os.Stderr)
	if err != nil {
		return nil, err
	}

	return &repo, nil
}

// GetCommit returns the commit hash for current repo HEAD
func (repo *Repository) GetCommit() (string, error) {
	repoDir := repo.Dir()
	if !check.DirExists(repoDir) {
		return "", fmt.Errorf("directory %s does not exist", repoDir)
	}

	cmd := exec.Command("git", "rev-parse", "HEAD")
	commit, err := check.Output(cmd, repoDir, check.EmptyEnv, os.Stderr)
	if err != nil {
		return "", err
	}

	return commit[:len(commit)-1], nil
}

// Checkout pulls from upstream and checkout to a commit hash.
func (repo *Repository) Checkout(commit string) error {
	repoDir := repo.Dir()
	if !check.DirExists(repoDir) {
		return fmt.Errorf("directory %s does not exist", repoDir)
	}

	cmd := exec.Command("git", "checkout", "-")
	check.Run(cmd, repoDir, check.EmptyEnv, os.Stdout, os.Stderr)

	cmd = exec.Command("git", "pull")
	err := check.Run(cmd, repoDir, check.EmptyEnv, os.Stdout, os.Stderr)
	if err != nil {
		return err
	}
	cmd = exec.Command("git", "checkout", commit)
	err = check.Run(cmd, repoDir, check.EmptyEnv, os.Stdout, os.Stderr)
	if err != nil {
		return err
	}

	return nil
}

// BisectStart starts a git bisect on a repository.
// It uses badCommit and goodCommits to narrow down the search path.
// Current head is used if badCommit is empty, and throws error if
// goodCommits is empty. It returns true if git bisect has ended.
func (repo *Repository) BisectStart(badCommit string, goodCommits []string) (bool, error) {
	if len(goodCommits) == 0 {
		return false, fmt.Errorf("No good commits provided")
	}
	repoDir := repo.Dir()
	if !check.DirExists(repoDir) {
		return false, fmt.Errorf("directory %s does not exist", repoDir)
	}

	if badCommit == "" {
		badCommit = "HEAD"
	}

	args := []string{"bisect", "start", badCommit}
	args = append(args, goodCommits...)

	cmd := exec.Command("git", args...)
	output, err := check.Output(cmd, repoDir, check.EmptyEnv, os.Stderr)
	if err != nil {
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
func (repo *Repository) BisectStep(testResult server.ResultType) (bool, error) {
	repoDir := repo.Dir()
	if !check.DirExists(repoDir) {
		return false, fmt.Errorf("directory %s does not exist", repoDir)
	}
	var step string
	switch testResult {
	case server.Pass:
		step = "good"
	case server.Failure:
		step = "bad"
	case server.Unknown:
		step = "skip"
	default:
		return false, fmt.Errorf("unexpect test result value")
	}

	cmd := exec.Command("git", "bisect", step)
	output, err := check.Output(cmd, repoDir, check.EmptyEnv, os.Stderr)
	if err != nil {
		return false, err
	}
	if strings.Contains(output, "is the first bad commit") {
		return true, nil
	}

	return false, nil
}

// BisectLog returns bisect log output.
func (repo *Repository) BisectLog() (string, error) {
	repoDir := repo.Dir()
	if !check.DirExists(repoDir) {
		return "", fmt.Errorf("directory %s does not exist", repoDir)
	}

	cmd := exec.Command("git", "bisect", "log")
	output, err := check.Output(cmd, repoDir, check.EmptyEnv, os.Stderr)
	if err != nil {
		return "", err
	}

	return output, nil
}

// BisectReset resets the current git bisect.
func (repo *Repository) BisectReset() error {
	repoDir := repo.Dir()
	if !check.DirExists(repoDir) {
		return fmt.Errorf("directory %s does not exist", repoDir)
	}

	cmd := exec.Command("git", "bisect", "reset")
	return check.Run(cmd, repoDir, check.EmptyEnv, os.Stdout, os.Stderr)
}

// BuildUpload builds the current kernel code and uploads image to GS.
// Script output is written into a given writer
func (repo *Repository) BuildUpload(gsBucket string, gsPath string, writer io.Writer) error {
	repoDir := repo.Dir()
	if !check.DirExists(repoDir) {
		return fmt.Errorf("directory %s does not exist", repoDir)
	}

	cmd := exec.Command(BuildUploadScript)

	env := map[string]string{
		"GS_BUCKET": gsBucket,
		"GS_PATH":   gsPath,
	}

	err := check.Run(cmd, repo.Dir(), env, writer, writer)

	return err
}

// Delete removes repo from local storage
func (repo *Repository) Delete() error {
	err := check.RemoveDir(repo.Dir())
	return err
}

// NewSimpleRepository clones a repo and checkout to commit without any caching and checking
func NewSimpleRepository(repoURL string, commit string) (*Repository, error) {
	err := check.CreateDir(RepoRootDir)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	r := Repository{
		url: repoURL,
		id:  id.String(),
	}

	cmd := exec.Command("git", "clone", repoURL, r.Dir())
	err = check.Run(cmd, RepoRootDir, check.EmptyEnv, os.Stdout, os.Stderr)
	if err != nil {
		return nil, err
	}

	cmd = exec.Command("git", "checkout", commit)
	err = check.Run(cmd, r.Dir(), check.EmptyEnv, os.Stdout, os.Stderr)
	if err != nil {
		return nil, err
	}

	return &r, nil
}

// Dir returns the repo directory
func (repo *Repository) Dir() string {
	return RepoRootDir + repo.id + "/"
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

// // MockRepo constructs a mock Repository struct without proper initialization.
// func MockRepo(repoURL string, id string, branch string, currCommit string, watching bool) Repository {
// 	return Repository{repoURL, id, branch, currCommit, watching}
// }
