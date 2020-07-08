package util

import (
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/google/uuid"
)

const (
	Rootdir          = "/root/repositories"
	FetchBuildScript = "/usr/local/lib/gce-fetch-build-kernel"
	checkInterval    = 10
)

type Repository struct {
	url        string
	id         string
	branch     string
	currCommit string
	watching   bool
}

// clone a repository into a unique directory with reference to linux
// base repository and checkout to a certain commit, branch or tag name
// return struct Repository
// only public repos are supported
func Clone(url string, commit string) Repository {
	id, _ := uuid.NewRandom()

	cmd := exec.Command(FetchBuildScript)
	env := map[string]string{
		"GIT_REPO": url,
		"REPO_ID":  id.String(),
		"COMMIT":   commit,
	}
	CheckRun(cmd, Rootdir, env)

	r := Repository{"", "", "", "", false}
	r.url = url
	r.id = id.String()

	// check whether we have a detached head
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branch := CheckOutput(cmd, r.GetDir(), EmptyEnv)
	branch = branch[:len(branch)-1]

	if branch == "HEAD" {
		r.currCommit = commit
	} else {
		r.currCommit = r.GetCommit()
		r.branch = branch
	}

	return r
}

// get the newest commit id on a local branch
// return the current commit if at a detached HEAD
func (r *Repository) GetCommit() string {
	dir := r.GetDir()
	stat, err := os.Stat(dir)
	if err != nil || !stat.IsDir() {
		log.Fatalf("directory %s does not exist!", dir)
	}
	cmd := exec.Command("git", "checkout", r.branch)
	CheckRun(cmd, dir, EmptyEnv)

	cmd = exec.Command("git", "rev-parse", "@")
	commit := CheckOutput(cmd, dir, EmptyEnv)
	return commit[:len(commit)-1]
}

// pull the newest code from upstream
func (r *Repository) Pull() {
	dir := r.GetDir()
	stat, err := os.Stat(dir)
	if err != nil || !stat.IsDir() {
		log.Fatalf("directory %s does not exist!", dir)
	}
	cmd := exec.Command("git", "pull")
	CheckRun(cmd, dir, EmptyEnv)
}

// watch a specified branch in a repo
// print newest commit id when upstream changes
// throw error if at a detached HEAD, i.e. r.branch is empty
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

func (r *Repository) GetDir() string {
	return Rootdir + "/" + r.id
}

// func FakeRepo(url string, id string, branch string, currCommit string, watching bool) Repository {
// 	return Repository{url, id, branch, currCommit, watching}
// }
