package util

import (
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/google/uuid"
)

// configurable constants for git utility functions
const (
	RepoRootDir      = "/root/repositories"
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
func Clone(url string, commit string) Repository {
	CreateDir(RepoRootDir)
	id, _ := uuid.NewRandom()

	cmd := exec.Command(FetchBuildScript)
	env := map[string]string{
		"GIT_REPO": url,
		"REPO_ID":  id.String(),
		"COMMIT":   commit,
	}
	err := CheckRun(cmd, RepoRootDir, env, os.Stdout, os.Stderr)
	Check(err)

	r := Repository{"", "", "", "", false}
	r.url = url
	r.id = id.String()

	// check whether we have a detached head
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branch, err := CheckOutput(cmd, r.getDir(), EmptyEnv, os.Stderr)
	Check(err)
	branch = branch[:len(branch)-1]

	if branch == "HEAD" {
		r.currCommit = commit
	} else {
		r.currCommit = r.GetCommit()
		r.branch = branch
	}

	return r
}

// GetCommit returns the newest commit id on a local branch without
// fetching from remote upstream.
// It returns the current commit if the repo is at a detached HEAD
func (r *Repository) GetCommit() string {
	dir := r.getDir()
	if !DirExists(dir) {
		log.Fatalf("directory %s does not exist!", dir)
	}
	cmd := exec.Command("git", "checkout", r.branch)
	err := CheckRun(cmd, dir, EmptyEnv, os.Stdout, os.Stderr)
	Check(err)

	cmd = exec.Command("git", "rev-parse", "@")
	commit, err := CheckOutput(cmd, dir, EmptyEnv, os.Stderr)
	Check(err)
	return commit[:len(commit)-1]
}

// Pull the newest code from upstream.
func (r *Repository) Pull() {
	dir := r.getDir()
	if !DirExists(dir) {
		log.Fatalf("directory %s does not exist!", dir)
	}
	cmd := exec.Command("git", "pull")
	err := CheckRun(cmd, dir, EmptyEnv, os.Stdout, os.Stderr)
	Check(err)
}

// Watch a specified branch and print the newest commit id when it
// detects code changes from upstream.
// Watch throws error if the repo is at a detached HEAD, indicated by
// r.branch == ""
func (r *Repository) Watch() {
	if r.watching {
		return
	}
	if r.branch == "" {
		log.Fatalf("repo has a detached HEAD %s\n", r.currCommit)
	}
	r.watching = true
	for {
		time.Sleep(checkInterval * time.Second)
		r.Pull()
		newCommit := r.GetCommit()
		if newCommit != r.currCommit {
			r.currCommit = newCommit
			log.Println("new commit detected")
		}
	}
}

func (r *Repository) getDir() string {
	return RepoRootDir + "/" + r.id
}

// MockRepo constructs a mock Repository struct without proper initialization.
func MockRepo(url string, id string, branch string, currCommit string, watching bool) Repository {
	return Repository{url, id, branch, currCommit, watching}
}
