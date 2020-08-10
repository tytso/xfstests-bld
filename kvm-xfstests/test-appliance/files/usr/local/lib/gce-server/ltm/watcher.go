package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"gce-server/util/check"
	"gce-server/util/email"
	"gce-server/util/gcp"
	"gce-server/util/git"
	"gce-server/util/logging"
	"gce-server/util/server"

	"github.com/sirupsen/logrus"
)

const (
	watchInterval = 1 * time.Minute
)

// GitWatcher watches a branch of a remote repo and detects new commits
type GitWatcher struct {
	testID    string
	searchKey watcherKey

	gsBucket       string
	bucketSubdir   string
	reportReceiver string
	testRequest    server.TaskRequest
	testHistory    []string

	repo *git.RemoteRepository
	done chan bool

	logDir     string
	resultsDir string
	logFile    string
	log        *logrus.Entry
}

// WatcherInfo exports watcher info.
type WatcherInfo struct {
	ID     string `json:"id"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	HEAD   string `json:"HEAD"`
}

type watcherKey struct {
	url    string
	branch string
}

// watcherMap indexes watchers by repo url and branch.
// Used for checking duplication and terminating a monitor.
var (
	watcherMap  = make(map[watcherKey]*GitWatcher)
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

	done := make(chan bool)
	repo, err := git.NewRemoteRepository(c.Options.GitRepo, c.Options.BranchName)
	check.Panic(err, log, "failed to initiate remote repo")

	c.ExtraOptions = &server.InternalOptions{
		TestID:    testID,
		Requester: server.LTMBuild,
	}

	watcher := &GitWatcher{
		testID:    testID,
		searchKey: searchKey,

		gsBucket:       gsBucket,
		bucketSubdir:   bucketSubdir,
		reportReceiver: c.Options.ReportEmail,
		testRequest:    c,
		testHistory:    []string{},

		repo:       repo,
		done:       done,
		logDir:     logDir,
		resultsDir: resultsDir,
		logFile:    logFile,
		log:        log,
	}

	watcherMap[searchKey] = watcher

	return watcher
}

// Run starts watching on a remote repo. The watcher checks remote HEAD
// periodically. If new commits are detected, it calls KCS to build a kernel
// and run a test.
func (watcher *GitWatcher) Run() {
	watcher.log.Debug("Starting watcher")
	defer watcher.Clean()
	var wg sync.WaitGroup

	ticker := time.NewTicker(watchInterval)

	wg.Add(1)
	go watcher.watch(ticker, &wg)
	wg.Wait()

	watcher.log.Debug("Watcher stopped")
}

func (watcher *GitWatcher) watch(ticker *time.Ticker, wg *sync.WaitGroup) {
	defer wg.Done()
	subject := fmt.Sprintf("xfstests LTM watcher failure " + watcher.testID)
	defer email.ReportFailure(watcher.log, watcher.logFile, watcher.reportReceiver, subject)

	start := time.Now()
	watcher.InitTest()

	for {
		select {
		case <-watcher.done:
			watcher.log.Info("Received terminating signal, generating watcher summary")
			watcher.Finish()
			return

		case <-ticker.C:
			watcher.log.WithField("time", time.Since(start).Round(time.Second)).Debug("Checking new commits")
			updated, err := watcher.repo.Update()
			check.Panic(err, watcher.log, "Failed to update repo")

			if updated {
				watcher.InitTest()
			}
		}
	}
}

// InitTest initiates a kernel building and testing using the current repo head.
func (watcher *GitWatcher) InitTest() {
	watcher.log.WithField("commit", watcher.repo.Head()).Info("initiating new build and test task")
	testID := watcher.testID + "-" + watcher.repo.Head()[:8]
	watcher.testHistory = append(watcher.testHistory, testID)
	watcher.testRequest.Options.CommitID = watcher.repo.Head()
	watcher.testRequest.ExtraOptions.TestID = testID

	go ForwardKCS(watcher.testRequest, watcher.testID)
}

// Finish fetches and aggregates the test result files.
// It generates a summary file and sends the email report.
func (watcher *GitWatcher) Finish() {
	gce, err := gcp.NewService(watcher.gsBucket)
	if !check.NoError(err, watcher.log, "Failed to connect to GCE service") {
		return
	}
	defer gce.Close()

	watcher.aggResults(gce)
	watcher.packResults(gce)
}

func (watcher *GitWatcher) aggResults(gce *gcp.Service) {
	watcher.log.Info("Fetching test results")
	file, err := os.Create(watcher.resultsDir + "report")
	if !check.NoError(err, watcher.log, "Failed to create file") {
		return
	}
	defer file.Close()

	info := watcher.Info()
	e, err := json.MarshalIndent(&info, "", "  ")
	if !check.NoError(err, watcher.log, "Failed to parse json") {
		return
	}
	fmt.Fprintf(file, "LTM watcher info:\n%s\n", e)

	for _, testID := range watcher.testHistory {
		fmt.Fprintf(file, "\n============TEST %s============\n", testID)
		reportFile, err := watcher.getResults(testID, gce)
		if err == nil {
			sourceFile, err := os.Open(reportFile)
			if check.NoError(err, watcher.log, "Failed to open file") {
				_, err = io.Copy(file, sourceFile)
				check.NoError(err, watcher.log, "Failed to copy file")

				sourceFile.Close()
			}
		}
		if err != nil {
			fmt.Fprintf(file, "No test results available, check prior emails or log for errors\n")
		}
	}
}

func (watcher *GitWatcher) getResults(testID string, gce *gcp.Service) (string, error) {
	prefix := fmt.Sprintf("%s/results.%s-%s", watcher.bucketSubdir, server.LTMUserName, testID)
	resultFiles, err := gce.GetFileNames(prefix)
	if !check.NoError(err, watcher.log, "Failed to get GS filenames") {
		return "", err
	}

	if len(resultFiles) >= 1 {
		watcher.log.WithField("resultURL", resultFiles[0]).Debug("Found result file url")

		url := fmt.Sprintf("gs://%s/%s", watcher.gsBucket, resultFiles[0])
		cmd := exec.Command("gce-xfstests", "get-results", "--unpack", url)
		cmdLog := watcher.log.WithField("cmd", cmd.String())
		w := cmdLog.Writer()
		defer w.Close()
		err := check.Run(cmd, check.RootDir, check.EmptyEnv, w, w)
		if !check.NoError(err, cmdLog, "Failed to run get-results") {
			return "", err
		}

		tmpResultsDir := fmt.Sprintf("/tmp/results-%s-%s", server.LTMUserName, testID)
		unpackedResultsDir := watcher.logDir + "results/" + testID + "/"

		if check.DirExists(tmpResultsDir) {
			os.RemoveAll(unpackedResultsDir)
			err = os.Rename(tmpResultsDir, unpackedResultsDir)
			if !check.NoError(err, watcher.log, "Failed to move dir") {
				return "", err
			}
		} else {
			return "", fmt.Errorf("Failed to find unpacked result files")
		}
		reportFile := unpackedResultsDir + "report"
		if !check.FileExists(reportFile) {
			return "", fmt.Errorf("test results found but failed to get report file")
		}
		return reportFile, nil
	}

	return "", fmt.Errorf("Failed to get test result")
}

func (watcher *GitWatcher) packResults(gce *gcp.Service) {
	watcher.log.Info("Packing test results")
	aggFile := fmt.Sprintf("%sresults.%s-%s-watcher", watcher.logDir, server.LTMUserName, watcher.testID)

	cmd := exec.Command("tar", "-cf", aggFile+".tar", "-C", watcher.resultsDir, ".")
	cmdLog := watcher.log.WithField("cmd", cmd.Args)
	w1 := cmdLog.Writer()
	defer w1.Close()
	err := check.Run(cmd, check.RootDir, check.EmptyEnv, w1, w1)
	if !check.NoError(err, cmdLog, "Failed to create tarball") {
		return
	}

	cmd = exec.Command("xz", "-6ef", aggFile+".tar")
	cmdLog = watcher.log.WithField("cmd", cmd.Args)
	w2 := cmdLog.Writer()
	defer w2.Close()
	err = check.Run(cmd, check.RootDir, check.EmptyEnv, w2, w2)
	if !check.NoError(err, cmdLog, "Failed to create xz compressed tarball") {
		return
	}

	watcher.log.Info("Removing separate results tarball")
	prefix := fmt.Sprintf("%s/results.%s-%s", watcher.bucketSubdir, server.LTMUserName, watcher.testID)
	_, err = gce.DeleteFiles(prefix)
	check.NoError(err, watcher.log, "Failed to delete file")

	watcher.log.Info("Uploading repacked results tarball")
	gsPath := fmt.Sprintf("%s/results.%s-%s-watcher.tar.xz", watcher.bucketSubdir, server.LTMUserName, watcher.testID)
	err = gce.UploadFile(aggFile+".tar.xz", gsPath)
	check.NoError(err, watcher.log, "Failed to upload results tarball")

	os.Remove(aggFile + ".tar.xz")
}

// Clean removes the watcher from watcherMap and performs other cleanup.
func (watcher *GitWatcher) Clean() {
	watcherLock.Lock()
	defer watcherLock.Unlock()
	watcher.log.Info("Cleaning up watcher resources")
	delete(watcherMap, watcher.searchKey)
	close(watcher.done)
	os.RemoveAll(watcher.resultsDir)
	logging.CloseLog(watcher.log)
}

// Info returns structured watcher information.
func (watcher *GitWatcher) Info() WatcherInfo {
	return WatcherInfo{
		ID:     watcher.testID,
		Repo:   watcher.testRequest.Options.GitRepo,
		Branch: watcher.testRequest.Options.BranchName,
		HEAD:   watcher.repo.Head(),
	}
}

// StopWatcher finds the running watcher on a given branch and terminate it.
// It panics if no matching monitor is found.
func StopWatcher(c server.TaskRequest) {
	searchKey := watcherKey{c.Options.GitRepo, c.Options.BranchName}
	if watcher, ok := watcherMap[searchKey]; ok {
		watcher.done <- true
		return
	}
	panic("Failed to find a monitor linked with given branch")
}

// WatcherStatus returns the info for active git watchers.
func WatcherStatus() []WatcherInfo {
	watcherLock.Lock()
	defer watcherLock.Unlock()
	infoList := []WatcherInfo{}
	for _, v := range watcherMap {
		infoList = append(infoList, v.Info())
	}

	return infoList
}
