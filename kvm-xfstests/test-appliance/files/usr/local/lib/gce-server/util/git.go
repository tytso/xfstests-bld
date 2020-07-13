package util

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/google/uuid"
)

// configurable constants for git utility functions
const (
	RepoRootDir      = "/root/repositories/"
	FetchBuildScript = "/usr/local/lib/gce-fetch-build-kernel"
	checkInterval    = 10
)

// Repository represents a git repository and its current states
type Repository struct {
	url        string
	id         string
	branch     string
	currCommit string
	watching   bool
}

/*
Clone a repository into a unique directory with reference to the linux
base repository. It then checkout to a certain commit, branch or tag name
and returns a Repository struct.
Only public repos are supported for now.
*/
func Clone(url string, commit string) (*Repository, error) {
	err := CreateDir(RepoRootDir)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(FetchBuildScript)
	env := map[string]string{
		"GIT_REPO": url,
		"REPO_ID":  id.String(),
		"COMMIT":   commit,
	}
	err = CheckRun(cmd, RepoRootDir, env, os.Stdout, os.Stderr)
	if err != nil {
		return nil, err
	}

	r := Repository{
		url:      url,
		id:       id.String(),
		watching: false,
	}

	// check whether we have a detached head
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branch, err := CheckOutput(cmd, r.Dir(), EmptyEnv, os.Stderr)
	if err != nil {
		return nil, err
	}

	branch = branch[:len(branch)-1]
	if branch == "HEAD" {
		r.currCommit = commit
	} else {
		r.currCommit, err = r.GetCommit()
		if err != nil {
			return nil, err
		}
		r.branch = branch
	}

	return &r, nil
}

// SimpleClone clones a repo and checkout to commit without any caching and checking
func SimpleClone(url string, commit string) (*Repository, error) {
	err := CreateDir(RepoRootDir)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	r := Repository{
		url:        url,
		id:         id.String(),
		currCommit: commit,
		watching:   false,
	}

	cmd := exec.Command("git", "clone", url, r.Dir())
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

// GetCommit returns the newest commit id on a local branch without
// fetching from remote upstream.
// It returns the current commit if the repo is at a detached HEAD
func (r *Repository) GetCommit() (string, error) {
	dir := r.Dir()
	if !DirExists(dir) {
		return "", fmt.Errorf("directory %s does not exist", dir)
	}
	cmd := exec.Command("git", "checkout", r.branch)
	err := CheckRun(cmd, dir, EmptyEnv, os.Stdout, os.Stderr)
	if err != nil {
		return "", err
	}

	cmd = exec.Command("git", "rev-parse", "@")
	commit, err := CheckOutput(cmd, dir, EmptyEnv, os.Stderr)
	if err != nil {
		return "", err
	}

	return commit[:len(commit)-1], nil
}

// Pull the newest code from upstream.
func (r *Repository) Pull() error {
	dir := r.Dir()
	if !DirExists(dir) {
		return fmt.Errorf("directory %s does not exist", dir)
	}
	cmd := exec.Command("git", "pull")
	err := CheckRun(cmd, dir, EmptyEnv, os.Stdout, os.Stderr)
	return err
}

// Watch a specified branch and print the newest commit id when it
// detects code changes from upstream.
// Watch throws error if the repo is at a detached HEAD, indicated by
// r.branch == ""
func (r *Repository) Watch() error {
	if r.watching {
		return nil
	}
	if r.branch == "" {
		return fmt.Errorf("repo has a detached HEAD %s", r.currCommit)
	}
	r.watching = true
	for {
		time.Sleep(checkInterval * time.Second)
		r.Pull()
		newCommit, err := r.GetCommit()
		if err != nil {
			return err
		}
		if newCommit != r.currCommit {
			r.currCommit = newCommit
			fmt.Println("new commit detected")
		}
	}
}

func (r *Repository) Dir() string {
	return RepoRootDir + r.id + "/"
}

// MockRepo constructs a mock Repository struct without proper initialization.
func MockRepo(url string, id string, branch string, currCommit string, watching bool) Repository {
	return Repository{url, id, branch, currCommit, watching}
}
