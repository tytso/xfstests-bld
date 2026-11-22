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

	repo      *git.RemoteRepository
	done      chan struct{}
	stopOnce  sync.Once
	cleanOnce sync.Once
	forwardWG sync.WaitGroup
	stopping  bool

	logDir     string
	resultsDir string
	logFile    string
	log        *logrus.Entry
}

// watcherMap indexes watchers by testID.
// Used for checking duplication and terminating a watcher.
var (
	watcherMap      = make(map[string]*GitWatcher)
	pendingWatchers = make(map[string]bool)
	watcherLock     sync.Mutex
	forwardKCSFunc  = ForwardKCS
)

func reserveWatcherID(testID string) bool {
	watcherLock.Lock()
	defer watcherLock.Unlock()
	if _, ok := watcherMap[testID]; ok || pendingWatchers[testID] {
		return false
	}
	pendingWatchers[testID] = true
	return true
}

// NewGitWatcher constructs a new git watcher from a watch request.
// It returns an error if testID is invalid or already linked with a watcher.
func NewGitWatcher(c server.TaskRequest, testID string) (*GitWatcher, error) {
	if err := check.ValidateTestID(testID); err != nil {
		return nil, fmt.Errorf("invalid testID: %w", err)
	}
	if c.Options != nil {
		if c.Options.BucketSubdir != "" {
			if err := check.ValidateBucketSubdir(c.Options.BucketSubdir); err != nil {
				return nil, fmt.Errorf("invalid bucket_subdir: %w", err)
			}
		}
		if c.Options.KConfig != "" {
			if err := check.ValidateGSURI(c.Options.KConfig); err != nil {
				return nil, fmt.Errorf("invalid kconfig: %w", err)
			}
		}
		if c.Options.Arch != "" {
			if err := check.ValidateArch(c.Options.Arch); err != nil {
				return nil, fmt.Errorf("invalid arch: %w", err)
			}
		}
		if c.Options.KConfigOpts != "" {
			if err := check.ValidateBuildOpts(c.Options.KConfigOpts); err != nil {
				return nil, fmt.Errorf("invalid kconfig_opts: %w", err)
			}
		}
		if c.Options.KbuildOpts != "" {
			if err := check.ValidateBuildOpts(c.Options.KbuildOpts); err != nil {
				return nil, fmt.Errorf("invalid kbuild_opts: %w", err)
			}
		}
		for _, addr := range []string{c.Options.ReportEmail, c.Options.ReportFailEmail} {
			if addr != "" {
				if err := check.ValidateEmailList(addr); err != nil {
					return nil, fmt.Errorf("invalid email option: %w", err)
				}
			}
		}
	}

	if !reserveWatcherID(testID) {
		return nil, fmt.Errorf("given testID is already linked with a watcher")
	}

	var log *logrus.Entry
	registered := false
	defer func() {
		if !registered {
			watcherLock.Lock()
			delete(pendingWatchers, testID)
			watcherLock.Unlock()
			if log != nil {
				logging.CloseLog(log)
			}
		}
	}()

	logDir := logging.LTMLogDir + testID + "/"
	err := check.CreateDir(logDir)
	if err != nil {
		return nil, err
	}

	logFile := logDir + "run.log"
	log = logging.InitLogger(logFile)
	log.Info("Initiating git watcher")

	resultsDir := logDir + "results/"
	err = check.CreateDir(resultsDir)
	check.Panic(err, log, "Failed to create dir")

	bucketSubdir, _ := gcp.GceConfig().Get("BUCKET_SUBDIR")
	if c.Options.BucketSubdir != "" {
		bucketSubdir = c.Options.BucketSubdir
	}
	if bucketSubdir == "" {
		bucketSubdir = "results"
	}
	err = check.ValidateBucketSubdir(bucketSubdir)
	check.Panic(err, log, "Invalid bucket_subdir")

	gsBucket, err := gcp.GceConfig().Get("GS_BUCKET")
	check.Panic(err, log, "Failed to get gs bucket config")

	origCmd, err := parser.DecodeCmd(c.CmdLine)
	check.Panic(err, log, "Failed to decode cmdline")

	done := make(chan struct{})
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

	watcherLock.Lock()
	delete(pendingWatchers, testID)
	watcherMap[testID] = watcher
	registered = true
	watcherLock.Unlock()

	return watcher, nil
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

	subject := "xfstests LTM watcher failure " + watcher.testID
	defer email.ReportFailure(watcher.log, watcher.logFile, watcher.reportFailReceiver, subject)

	checkTicker := time.NewTicker(checkInterval)
	defer checkTicker.Stop()
	aggTicker := time.NewTicker(aggInterval)
	defer aggTicker.Stop()

	start := time.Now()
	if !watcher.testRequest.Options.WatchSkipInitial {
		watcher.InitTest()
	} else {
		watcher.log.Info("Skipping initial test run as requested")
	}

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

func (watcher *GitWatcher) recordNewBuild(commit string) string {
	watcher.historyLock.Lock()
	defer watcher.historyLock.Unlock()

	watcher.buildID++
	log := watcher.log.WithFields(logrus.Fields{
		"buildID": watcher.buildID,
		"commit":  commit,
	})
	log.Info("initiating new build and test task")
	testID := fmt.Sprintf("%s-%04d", watcher.testID, watcher.buildID)

	shortCommit := commit
	if len(shortCommit) > 12 {
		shortCommit = shortCommit[:12]
	}

	watcher.testHistory = append(watcher.testHistory, server.TestInfo{
		TestID:     testID,
		Commit:     shortCommit,
		UpdateTime: time.Now().Format(time.Stamp),
		Status:     "running",
	})
	return testID
}

func (watcher *GitWatcher) cloneTaskRequest(commit string, testID string) server.TaskRequest {
	var optsCopy server.UserOptions
	if watcher.testRequest.Options != nil {
		optsCopy = *watcher.testRequest.Options
	}
	optsCopy.CommitID = commit

	var extraCopy server.InternalOptions
	if watcher.testRequest.ExtraOptions != nil {
		extraCopy = *watcher.testRequest.ExtraOptions
	}
	extraCopy.TestID = testID

	return server.TaskRequest{
		CmdLine:      watcher.testRequest.CmdLine,
		Options:      &optsCopy,
		ExtraOptions: &extraCopy,
	}
}

// InitTest initiates a kernel building and testing using the current repo head.
func (watcher *GitWatcher) InitTest() {
	commit := watcher.repo.Head()
	testID := watcher.recordNewBuild(commit)
	req := watcher.cloneTaskRequest(commit, testID)

	watcher.forwardWG.Add(1)
	go func(req server.TaskRequest, id string) {
		defer watcher.forwardWG.Done()
		if err := forwardKCSFunc(req, id); err != nil {
			watcher.log.WithError(err).Error("Failed to forward request to KCS")
		}
	}(req, testID)
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

func (watcher *GitWatcher) stop() {
	watcher.stopOnce.Do(func() {
		close(watcher.done)
	})
}

// Clean removes the watcher from watcherMap and performs other cleanup.
func (watcher *GitWatcher) Clean() {
	watcher.cleanOnce.Do(func() {
		watcherLock.Lock()
		watcher.stopping = true
		delete(watcherMap, watcher.testID)
		watcherLock.Unlock()

		watcher.stop()
		watcher.forwardWG.Wait()
		if watcher.log != nil {
			watcher.log.Info("Cleaning up watcher resources")
		}
		if watcher.resultsDir != "" {
			os.RemoveAll(watcher.resultsDir)
		}
		if watcher.log != nil {
			logging.CloseLog(watcher.log)
		}
	})
}

// Info returns structured watcher information.
func (watcher *GitWatcher) Info() server.WatcherInfo {
	watcher.historyLock.Lock()
	tests := watcher.testHistory
	if len(tests) > historyLength {
		tests = tests[len(tests)-historyLength:]
	}
	testsCopy := make([]server.TestInfo, len(tests))
	copy(testsCopy, tests)
	packsCopy := make([]string, len(watcher.packHistory))
	copy(packsCopy, watcher.packHistory)
	watcher.historyLock.Unlock()

	var repoURL, branch string
	if watcher.testRequest.Options != nil {
		repoURL = watcher.testRequest.Options.GitRepo
		branch = watcher.testRequest.Options.BranchName
	}

	return server.WatcherInfo{
		ID:      watcher.testID,
		Command: watcher.origCmd,
		Repo:    repoURL,
		Branch:  branch,
		HEAD:    watcher.repo.Head(),
		Tests:   testsCopy,
		Packs:   packsCopy,
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
// It returns an error if unwatch ID is invalid or no matching watcher is found.
func StopWatcher(c server.TaskRequest) error {
	if err := check.ValidateTestID(c.Options.UnWatch); err != nil {
		return fmt.Errorf("invalid unwatch ID: %w", err)
	}
	watcherLock.Lock()
	watcher, ok := watcherMap[c.Options.UnWatch]
	if !ok || watcher.stopping {
		watcherLock.Unlock()
		return fmt.Errorf("no active watcher with specified ID")
	}
	watcher.stopping = true
	watcher.stop()
	watcherLock.Unlock()
	return nil
}

// WatcherStatus returns the info for active git watchers.
func WatcherStatus() []server.WatcherInfo {
	watcherLock.Lock()
	watchers := make([]*GitWatcher, 0, len(watcherMap))
	for _, v := range watcherMap {
		watchers = append(watchers, v)
	}
	watcherLock.Unlock()

	infoList := make([]server.WatcherInfo, 0, len(watchers))
	for _, v := range watchers {
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
