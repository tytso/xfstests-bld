package main

import (
	"fmt"
	"gce-server/logging"
	"gce-server/server"
	"gce-server/util"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	timeout  = 1 * time.Hour
	duration = 10 * time.Second
)

// GitWatcher watches a branch of a remote repo and detects new commits
type GitWatcher struct {
	testID         string
	url            string
	branch         string
	head           string
	reportReceiver string
	testRequest    server.TaskRequest

	done    <-chan bool
	logFile string
	log     *logrus.Entry
}

type watcherKey struct {
	url    string
	branch string
}

// watcherLookup relates the git repo and branch name to a watcher.
// Used for checking duplication and terminating a monitor.
var watcherLookup = make(map[watcherKey]chan<- bool)

// NewGitWatcher constructs a new git watcher from a watch request.
// It panics if there is already a monitor running on this branch.
func NewGitWatcher(c server.TaskRequest, testID string) *GitWatcher {
	searchKey := watcherKey{c.Options.GitRepo, c.Options.BranchName}
	if _, ok := watcherLookup[searchKey]; ok {
		panic("Given branch is already linked with a monitor")
	}

	logDir := logging.LTMLogDir + testID + "/"
	err := util.CreateDir(logDir)
	if err != nil {
		panic(err)
	}

	logFile := logDir + "run.log"
	log := logging.InitLogger(logFile)
	log.Info("Initiating git watcher")

	done := make(chan bool)
	watcher := GitWatcher{
		testID:         testID,
		url:            c.Options.GitRepo,
		branch:         c.Options.BranchName,
		head:           "",
		reportReceiver: c.Options.ReportEmail,
		testRequest:    c,

		done:    done,
		logFile: logFile,
		log:     log,
	}

	_, err = getHead(watcher.url, watcher.branch)
	logging.CheckPanic(err, log, "Failed to get HEAD")

	watcherLookup[searchKey] = done

	return &watcher
}

// Run starts watching on a remote repo. The watcher checks remote HEAD
// periodically. If new commits are detected, it calls KCS to build a kernel
// and run a test.
func (watcher *GitWatcher) Run() {
	watcher.log.Debug("Starting watcher")
	defer logging.CloseLog(watcher.log)
	var wg sync.WaitGroup

	ticker := time.NewTicker(duration)

	wg.Add(1)
	go watcher.watch(ticker, &wg)
	wg.Wait()

	watcher.log.Debug("Watcher stopped")
}

func (watcher *GitWatcher) watch(ticker *time.Ticker, wg *sync.WaitGroup) {
	defer wg.Done()
	subject := fmt.Sprintf("xfstests LTM watcher failure " + watcher.testID)
	defer util.ReportFailure(watcher.log, watcher.logFile, watcher.reportReceiver, subject)

	for {
		select {
		case <-watcher.done:
			watcher.log.Info("Received terminating signal, stopping monitor")
			return
		case t := <-ticker.C:
			watcher.log.WithField("time", t).Debug("Checking new commits")
			newCommit, err := getHead(watcher.url, watcher.branch)
			logging.CheckPanic(err, watcher.log, "Failed to get HEAD")

			if newCommit != watcher.head {
				watcher.log.WithField("commit", newCommit).Info("New commit detected, initiating build")
				testID := watcher.testID + "-" + util.GetTimeStamp()
				watcher.testRequest.Options.CommitID = newCommit

				if logging.DEBUG {
					go MockStartBuild(watcher.testRequest, testID)
				} else {
					go StartBuild(watcher.testRequest, testID)
				}

				watcher.head = newCommit
			}
		}
	}
}

// getHead retrives the commit hash of the HEAD on a branch
func getHead(url string, branch string) (string, error) {
	cmd := exec.Command("git", "ls-remote", "--heads", "--quiet", "--exit-code", url, branch)
	output, err := util.CheckOutput(cmd, util.RootDir, util.EmptyEnv, os.Stderr)
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

// StopWatcher finds the running watcher on a given branch and terminate it.
// It panics if no matching monitor is found.
func StopWatcher(c server.TaskRequest) {
	searchKey := watcherKey{c.Options.GitRepo, c.Options.BranchName}
	if done, ok := watcherLookup[searchKey]; ok {
		done <- true
		delete(watcherLookup, searchKey)
		return
	}
	panic("Failed to find a monitor linked with given branch")
}
