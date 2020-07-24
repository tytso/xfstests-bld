package main

import (
	"fmt"
	"sync"
	"time"

	"gce-server/util/check"
	"gce-server/util/email"
	"gce-server/util/git"
	"gce-server/util/logging"
	"gce-server/util/mymath"
	"gce-server/util/server"

	"github.com/sirupsen/logrus"
)

const (
	timeout  = 1 * time.Hour
	duration = 10 * time.Second
)

// GitWatcher watches a branch of a remote repo and detects new commits
type GitWatcher struct {
	testID         string
	reportReceiver string
	testRequest    server.TaskRequest

	repo    *git.RemoteRepository
	done    <-chan bool
	logFile string
	log     *logrus.Entry
}

type watcherKey struct {
	url    string
	branch string
}

// watcherMap indexes watcheres by repo url and branch.
// Used for checking duplication and terminating a monitor.
var (
	watcherMap  = make(map[watcherKey]chan<- bool)
	watcherLock sync.Mutex
)

// NewGitWatcher constructs a new git watcher from a watch request.
// It panics if there is already a monitor running on this branch.
func NewGitWatcher(c server.TaskRequest, testID string) *GitWatcher {
	watcherLock.Lock()
	defer watcherLock.Unlock()
	searchKey := watcherKey{c.Options.GitRepo, c.Options.BranchName}
	if _, ok := watcherMap[searchKey]; ok {
		panic("Given branch is already linked with a monitor")
	}

	logDir := logging.LTMLogDir + testID + "/"
	err := check.CreateDir(logDir)
	if err != nil {
		panic(err)
	}

	logFile := logDir + "run.log"
	log := logging.InitLogger(logFile).WithField("testID", testID)
	log.Info("Initiating git watcher")

	done := make(chan bool)
	repo, err := git.NewRemoteRepository(c.Options.GitRepo, c.Options.BranchName)
	check.Panic(err, log, "failed to initiate remote repo")

	watcher := GitWatcher{
		testID:         testID,
		reportReceiver: c.Options.ReportEmail,
		testRequest:    c,

		repo:    repo,
		done:    done,
		logFile: logFile,
		log:     log,
	}

	watcherMap[searchKey] = done

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
	defer email.ReportFailure(watcher.log, watcher.logFile, watcher.reportReceiver, subject)

	watcher.log.Info("Initiating build at watcher launch")
	watcher.testRequest.Options.CommitID = watcher.repo.Head()

	go ForwardKCS(watcher.testRequest, watcher.testID)

	start := time.Now()

	for {
		select {
		case <-watcher.done:
			watcher.log.Info("Received terminating signal, stopping monitor")
			return

		case <-ticker.C:
			watcher.log.WithField("time", time.Since(start)).Debug("Checking new commits")
			updated, err := watcher.repo.Update()
			check.Panic(err, watcher.log, "Failed to update repo")

			if updated {
				watcher.log.WithField("commit", watcher.repo.Head()).Info("New commit detected, initiating build")
				testID := watcher.testID + "-" + mymath.GetTimeStamp()
				watcher.testRequest.Options.CommitID = watcher.repo.Head()

				go ForwardKCS(watcher.testRequest, testID)
			}
		}
	}
}

// StopWatcher finds the running watcher on a given branch and terminate it.
// It panics if no matching monitor is found.
func StopWatcher(c server.TaskRequest) {
	watcherLock.Lock()
	defer watcherLock.Unlock()
	searchKey := watcherKey{c.Options.GitRepo, c.Options.BranchName}
	if done, ok := watcherMap[searchKey]; ok {
		done <- true
		delete(watcherMap, searchKey)
		return
	}
	panic("Failed to find a monitor linked with given branch")
}
