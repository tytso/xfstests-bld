package util

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/google/uuid"
)

// configurable constants for git utility functions
const (
	RepoRootDir = "/root/repositories/"
	RefRepoDir  = RepoRootDir + "linux.reference"
	RefRepoURL  = "https://git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git"
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
	err := CreateDir(RepoRootDir)
	if err != nil {
		panic(err)
	}
}

// NewRepository clones a repository with reference to a base repo.
// The repo directory is named as id under RepoRootDir.
// Do not overwrite directory if already exists.
func NewRepository(id string, repoURL string) (*Repository, error) {
	if id == "" {
		return nil, fmt.Errorf("repo id not specified")
	}
	if !DirExists(RefRepoDir) {
		cmd := exec.Command("git", "clone", "--mirror", RefRepoURL, RefRepoDir)
		err := CheckRun(cmd, RootDir, EmptyEnv, os.Stdout, os.Stderr)
		if err != nil {
			return nil, err
		}
	}

	repo := Repository{
		id:  id,
		url: repoURL,
	}

	repoDir := repo.Dir()
	if DirExists(repoDir) {
		return &repo, nil
	}

	cmd := exec.Command("git", "clone", "--reference", RefRepoDir, repoURL, repoDir)
	err := CheckRun(cmd, RootDir, EmptyEnv, os.Stdout, os.Stderr)
	if err != nil {
		return nil, err
	}

	return &repo, nil
}

// GetCommit returns the commit hash for current repo HEAD
func (repo *Repository) GetCommit() (string, error) {
	repoDir := repo.Dir()
	if !DirExists(repoDir) {
		return "", fmt.Errorf("directory %s does not exist", repoDir)
	}

	cmd := exec.Command("git", "rev-parse", "HEAD")
	commit, err := CheckOutput(cmd, repoDir, EmptyEnv, os.Stderr)
	if err != nil {
		return "", err
	}

	return commit[:len(commit)-1], nil
}

// Checkout pulls from upstream and checkout to a commit hash.
func (repo *Repository) Checkout(commit string) error {
	repoDir := repo.Dir()
	if !DirExists(repoDir) {
		return fmt.Errorf("directory %s does not exist", repoDir)
	}

	cmd := exec.Command("git", "checkout", "-")
	CheckRun(cmd, repoDir, EmptyEnv, os.Stdout, os.Stderr)

	cmd = exec.Command("git", "pull")
	err := CheckRun(cmd, repoDir, EmptyEnv, os.Stdout, os.Stderr)
	if err != nil {
		return err
	}
	cmd = exec.Command("git", "checkout", commit)
	err = CheckRun(cmd, repoDir, EmptyEnv, os.Stdout, os.Stderr)
	if err != nil {
		return err
	}

	return nil
}

// BuildUpload builds the current kernel code and uploads image to GS.
// Script output is written into a given writer
func (repo *Repository) BuildUpload(gsBucket string, gsPath string, writer io.Writer) error {
	repoDir := repo.Dir()
	if !DirExists(repoDir) {
		return fmt.Errorf("directory %s does not exist", repoDir)
	}

	cmd := exec.Command(BuildUploadScript)

	env := map[string]string{
		"GS_BUCKET": gsBucket,
		"GS_PATH":   gsPath,
	}

	err := CheckRun(cmd, repo.Dir(), env, writer, writer)

	return err
}

// Delete removes repo from local storage
func (repo *Repository) Delete() error {
	err := RemoveDir(repo.Dir())
	return err
}

// NewSimpleRepository clones a repo and checkout to commit without any caching and checking
func NewSimpleRepository(repoURL string, commit string) (*Repository, error) {
	err := CreateDir(RepoRootDir)
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
	err = CheckRun(cmd, RepoRootDir, EmptyEnv, os.Stdout, os.Stderr)
	if err != nil {
		return nil, err
	}

	cmd = exec.Command("git", "checkout", commit)
	err = CheckRun(cmd, r.Dir(), EmptyEnv, os.Stdout, os.Stderr)
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
	output, err := CheckOutput(cmd, RootDir, EmptyEnv, os.Stderr)
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

// ParseGitURL transforms a git url into a human readable directory string
// Format is hostname - last two parts of path - last 4 byte of md5 sum
// Clone with ssh key is not supported
func ParseGitURL(repoURL string) (string, error) {
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
