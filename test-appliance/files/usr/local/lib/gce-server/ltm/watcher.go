package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"thunk.org/gce-server/util/check"
	"thunk.org/gce-server/util/email"
	"thunk.org/gce-server/util/gcp"
	"thunk.org/gce-server/util/git"
	"thunk.org/gce-server/util/logging"
	"thunk.org/gce-server/util/parser"
	"thunk.org/gce-server/util/server"

	"github.com/sirupsen/logrus"
)

const (
	// checkInterval defines the interval to check for new commits.
	checkInterval = 1 * time.Minute
	// aggInterval defines the interval to aggregate results.
	aggInterval = 7 * 24 * time.Hour
	// aggMinCount sets the minimum existing tests to trigger tidy up.
	aggMinCount = 10
	// historyLength sets the length of testHistory return by ltm-info.
	historyLength = 10
)

// GitWatcher watches a branch of a remote repo and detects new commits
type GitWatcher struct {
	testID  string
	origCmd string

	gsBucket           string
	bucketSubdir       string
	reportReceiver     string
	reportFailReceiver string
	testRequest        server.TaskRequest
	testHistory        []server.TestInfo
	packHistory        []string
	historyLock        sync.Mutex
	buildID            int

	repo *git.RemoteRepository
	done chan bool

	logDir     string
	resultsDir string
	logFile    string
	log        *logrus.Entry
}

// watcherMap indexes watchers by testID.
// Used for checking duplication and terminating a watcher.
var (
	watcherMap  = make(map[string]*GitWatcher)
	watcherLock sync.Mutex
)

// NewGitWatcher constructs a new git watcher from a watch request.
// It panics if there is already a watcher running on this branch.
func NewGitWatcher(c server.TaskRequest, testID string) *GitWatcher {
	watcherLock.Lock()
	defer watcherLock.Unlock()
	if _, ok := watcherMap[testID]; ok {
		panic("Given testID is already linked with a watcher")
	}

	logDir := logging.LTMLogDir + testID + "/"
	err := check.CreateDir(logDir)
	if err != nil {
		panic(err)
	}

	logFile := logDir + "run.log"
	log := logging.InitLogger(logFile)
	log.Info("Initiating git watcher")

	resultsDir := logDir + "results/"
	err = check.CreateDir(resultsDir)
	check.Panic(err, log, "Failed to create dir")

	bucketSubdir, _ := gcp.GceConfig.Get("BUCKET_SUBDIR")
	if c.Options.BucketSubdir != "" {
		bucketSubdir = c.Options.BucketSubdir
	}
	if bucketSubdir == "" {
		bucketSubdir = "results"
	}

	gsBucket, err := gcp.GceConfig.Get("GS_BUCKET")
	check.Panic(err, log, "Failed to get gs bucket config")

	origCmd, err := parser.DecodeCmd(c.CmdLine)
	check.Panic(err, log, "Failed to decode cmdline")

	done := make(chan bool)
	repo, err := git.NewRemoteRepository(c.Options.GitRepo, c.Options.BranchName)
	check.Panic(err, log, "failed to initiate remote repo")

	c.ExtraOptions = &server.InternalOptions{
		TestID:    testID,
		Requester: server.LTMBuild,
	}

	watcher := &GitWatcher{
		testID:  testID,
		origCmd: origCmd,

		gsBucket:           gsBucket,
		bucketSubdir:       bucketSubdir,
		reportReceiver:     c.Options.ReportEmail,
		reportFailReceiver: c.Options.ReportFailEmail,
		testRequest:        c,
		testHistory:        []server.TestInfo{},
		packHistory:        []string{},
		buildID:            0,

		repo:       repo,
		done:       done,
		logDir:     logDir,
		resultsDir: resultsDir,
		logFile:    logFile,
		log:        log,
	}

	watcherMap[testID] = watcher

	return watcher
}

// Run starts watching on a remote repo. The watcher checks remote HEAD
// periodically. If new commits are detected, it calls KCS to build a kernel
// and run a test.
func (watcher *GitWatcher) Run() {
	watcher.log.Debug("Starting watcher")
	defer watcher.Clean()

	watcher.watch()

	watcher.log.Debug("Watcher stopped")
}

func (watcher *GitWatcher) watch() {
	var runonce bool
	var skip, skipAmount int

	subject := fmt.Sprintf("xfstests LTM watcher failure " + watcher.testID)
	defer email.ReportFailure(watcher.log, watcher.logFile, watcher.reportFailReceiver, subject)

	checkTicker := time.NewTicker(checkInterval)
	defer checkTicker.Stop()
	aggTicker := time.NewTicker(aggInterval)
	defer aggTicker.Stop()

	start := time.Now()
	watcher.InitTest()

	for {
		select {
		case <-watcher.done:
			watcher.log.Info("Received terminating signal, generating watcher summary")
			return

		case <-checkTicker.C:
			if skip > 0 {
				skip--
				continue
			}
			watcher.log.WithField("time", time.Since(start).Round(time.Second)).Debug("Checking for new commits")
			updated, err := watcher.repo.Update()
			if err != nil {
				if !runonce {
					check.Panic(err, watcher.log, "Failed to update repo")
				}
				if skipAmount > 0 {
					if skipAmount < 32 {
						skipAmount *= 2
					}
				} else {
					skipAmount = 1
				}
				skip = skipAmount
				watcher.log.WithField("skip", skip).Debug("Failed to update repo, retrying")
				continue
			}
			runonce = true
			skipAmount = 0
			if updated {
				watcher.InitTest()
			}

		case <-aggTicker.C:
			watcher.tidyUp()
		}

	}
}

// InitTest initiates a kernel building and testing using the current repo head.
func (watcher *GitWatcher) InitTest() {
	watcher.historyLock.Lock()
	watcher.buildID++
	log := watcher.log.WithFields(logrus.Fields{
		"buildID": watcher.buildID,
		"commit":  watcher.repo.Head(),
	})
	log.Info("initiating new build and test task")
	testID := fmt.Sprintf("%s-%04d", watcher.testID, watcher.buildID)

	watcher.testHistory = append(watcher.testHistory, server.TestInfo{
		TestID:     testID,
		Commit:     watcher.repo.Head()[:12],
		UpdateTime: time.Now().Format(time.Stamp),
		Status:     "running",
	})
	watcher.historyLock.Unlock()

	watcher.testRequest.Options.CommitID = watcher.repo.Head()
	watcher.testRequest.ExtraOptions.TestID = testID

	go ForwardKCS(watcher.testRequest, watcher.testID)
}

// tidyUp used to clean up the GCS bucket by fetching and aggregating
// the test result files periodically.  The problem is that combined
// tar file would very quickly huge, and it would take a huge amount
// of time to repack the tar file.   It also makes it harder to fetch
// the test artifacts tarball, so let's just drop aggregation.
// We'll save the tidyUp hook since eventually we might want to
// shorten the the test history to save memory (we only return
// last ten results anyway).
func (watcher *GitWatcher) tidyUp() {
	watcher.historyLock.Lock()
	defer watcher.historyLock.Unlock()
	watcher.log.Info("Tidy up the GCS results")
}

// Clean removes the watcher from watcherMap and performs other cleanup.
func (watcher *GitWatcher) Clean() {
	watcherLock.Lock()
	defer watcherLock.Unlock()
	watcher.log.Info("Cleaning up watcher resources")
	delete(watcherMap, watcher.testID)
	close(watcher.done)
	os.RemoveAll(watcher.resultsDir)
	logging.CloseLog(watcher.log)
}

// Info returns structured watcher information.
func (watcher *GitWatcher) Info() server.WatcherInfo {
	watcher.historyLock.Lock()
	defer watcher.historyLock.Unlock()
	tests := watcher.testHistory
	if len(tests) > historyLength {
		tests = tests[len(tests)-historyLength:]
	}
	return server.WatcherInfo{
		ID:      watcher.testID,
		Command: watcher.origCmd,
		Repo:    watcher.testRequest.Options.GitRepo,
		Branch:  watcher.testRequest.Options.BranchName,
		HEAD:    watcher.repo.Head(),
		Tests:   tests,
		Packs:   watcher.packHistory,
	}
}

// UpdateTest updates the info about a test.
func (watcher *GitWatcher) UpdateTest(testID string, testResult server.ResultType) {
	watcher.historyLock.Lock()
	defer watcher.historyLock.Unlock()
	watcher.log.WithField("testID", testID).Info("Updating test results")

	for i, test := range watcher.testHistory {
		if test.TestID == testID {
			watcher.testHistory[i].UpdateTime = time.Now().Format(time.Stamp)
			watcher.testHistory[i].Status = testResult.String()
			return
		}
	}
	watcher.log.WithField("testID", testID).Warn("testID not found in watcher history")
}

// StopWatcher finds the running watcher on a given branch and terminate it.
// It panics if no matching watcher is found.
func StopWatcher(c server.TaskRequest) {
	if watcher, ok := watcherMap[c.Options.UnWatch]; ok {
		watcher.done <- true
		return
	}
	panic("No active watcher with ID " + c.Options.UnWatch)
}

// WatcherStatus returns the info for active git watchers.
func WatcherStatus() []server.WatcherInfo {
	watcherLock.Lock()
	defer watcherLock.Unlock()
	infoList := []server.WatcherInfo{}
	for _, v := range watcherMap {
		infoList = append(infoList, v.Info())
	}
	sort.Slice(infoList, func(i, j int) bool {
		return infoList[i].ID < infoList[j].ID
	})
	return infoList
}

// UpdateWatcherTest attempts to find update the test info for a watcher test.
// It does nothing if testID is not related to any watcher.
func UpdateWatcherTest(testID string, testResult server.ResultType) {
	watcherLock.Lock()
	defer watcherLock.Unlock()

	baseID := strings.Split(testID, "-")[0]
	if watcher, ok := watcherMap[baseID]; ok {
		watcher.UpdateTest(testID, testResult)
	}
}
