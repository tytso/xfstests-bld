package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
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

// GitBisector performs a git bisect operation on a repo branch.
// Each bisector keeps a unique repository to save the states for bisect
// progress.
type GitBisector struct {
	testID  string
	origCmd string

	gsBucket       string
	bucketSubdir   string
	reportReceiver string
	testRequest    server.TaskRequest
	testHistory    []string

	repo        *git.Repository
	opMu        sync.Mutex
	stateMu     sync.RWMutex
	finished    bool
	cleaned     bool
	activeOps   int
	badCommit   string
	goodCommits []string
	lastActive  time.Time
	done        chan struct{}
	cleanOnce   sync.Once

	logDir     string
	resultsDir string
	log        *logrus.Entry
}

const (
	// bisectorTimeout defines the max idle time before a bisector get cleaned.
	bisectorTimeout = 4 * time.Hour
	checkInterval   = 5 * time.Minute
)

// bisectorMap indexes bisectors by testID which are guaranteed to be unique.
var (
	bisectorMap      = make(map[string]*GitBisector)
	pendingBisectors = make(map[string]bool)
	bisectorLock     sync.Mutex
)

// NewGitBisector constructs a new git bisect manager from a bisect request.
// The repo is initialized with a git bisect session.
// Creates a monitor goroutine that removes expired bisectors.
func NewGitBisector(c server.TaskRequest, testID string) (*GitBisector, error) {
	if err := check.ValidateTestID(testID); err != nil {
		return nil, fmt.Errorf("invalid testID: %w", err)
	}
	if c.Options == nil {
		return nil, fmt.Errorf("missing options in bisect request")
	}
	if c.Options.BadCommit != "" {
		if err := check.ValidateGitRef(c.Options.BadCommit); err != nil {
			return nil, fmt.Errorf("invalid badCommit: %w", err)
		}
	}
	if c.Options.GoodCommit == "" {
		return nil, fmt.Errorf("missing goodCommit in bisect request")
	}
	goodCommits := strings.Split(c.Options.GoodCommit, "|")
	for _, rev := range goodCommits {
		if err := check.ValidateGitRef(rev); err != nil {
			return nil, fmt.Errorf("invalid goodCommit %q: %w", rev, err)
		}
	}

	logDir := logging.KCSLogDir + testID + "/"
	err := check.CreateDir(logDir)
	if err != nil {
		return nil, err
	}

	logFile := logDir + "run.log"
	log := logging.InitLogger(logFile)
	initOK := false
	defer func() {
		if !initOK {
			logging.CloseLog(log)
		}
	}()
	log.Info("Initiating git bisector")

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

	gsBucket, err := gcp.GceConfig().Get("GS_BUCKET")
	check.Panic(err, log, "Failed to get gs bucket config")

	origCmd, err := parser.DecodeCmd(c.CmdLine)
	check.Panic(err, log, "Failed to decode cmdline")

	w := log.WithField("cmd", "bisectInit").Writer()
	defer w.Close()

	repo, err := git.NewBisectRepository(testID, c.Options.GitRepo, w)
	check.Panic(err, log, "Failed to clone repo")

	badCommit := c.Options.BadCommit

	bisector := GitBisector{
		testID:  testID,
		origCmd: origCmd,

		gsBucket:       gsBucket,
		bucketSubdir:   bucketSubdir,
		reportReceiver: c.Options.ReportEmail,
		testRequest:    c,
		testHistory:    []string{},

		repo:        repo,
		finished:    false,
		badCommit:   badCommit,
		goodCommits: goodCommits,
		lastActive:  time.Now(),
		done:        make(chan struct{}),

		logDir:     logDir,
		resultsDir: resultsDir,
		log:        log,
	}

	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()
		for {
			select {
			case <-bisector.done:
				return
			case <-ticker.C:
				if bisector.CheckActive() {
					return
				}
			}
		}
	}()

	initOK = true
	return &bisector, nil
}

func (bisector *GitBisector) touch() {
	bisector.stateMu.Lock()
	bisector.lastActive = time.Now()
	bisector.stateMu.Unlock()
}

func (bisector *GitBisector) beginOp() bool {
	bisector.stateMu.Lock()
	defer bisector.stateMu.Unlock()
	if bisector.cleaned {
		return false
	}
	bisector.activeOps++
	bisector.lastActive = time.Now()
	return true
}

func (bisector *GitBisector) endOp() {
	bisector.stateMu.Lock()
	bisector.activeOps--
	bisector.lastActive = time.Now()
	bisector.stateMu.Unlock()
}

// Start starts the bisect.
func (bisector *GitBisector) Start() {
	bisector.touch()
	bisector.log.Debug("Git bisect Start")

	w := bisector.log.WithField("cmd", "bisectStart").Writer()
	defer w.Close()

	bisector.log.Debug("Validate commits")

	bisector.stateMu.RLock()
	badCommit := bisector.badCommit
	goodCommits := make([]string, len(bisector.goodCommits))
	copy(goodCommits, bisector.goodCommits)
	bisector.stateMu.RUnlock()

	valid, err := bisector.repo.Valid(badCommit, w)
	if !valid && !strings.HasPrefix(badCommit, "origin/") {
		badCommit = "origin/" + badCommit
		valid, err = bisector.repo.Valid(badCommit, w)
	}
	check.Panic(err, bisector.log, "Failed to validate badCommit")

	for i, rev := range goodCommits {
		valid, err = bisector.repo.Valid(rev, w)
		if !valid && !strings.HasPrefix(rev, "origin/") {
			goodCommits[i] = "origin/" + rev
			valid, err = bisector.repo.Valid(goodCommits[i], w)
		}
		check.Panic(err, bisector.log, "Failed to validate goodCommit")
	}

	finished, err := bisector.repo.BisectStart(badCommit, goodCommits, w)
	check.Panic(err, bisector.log, "Failed to start bisect")

	bisector.stateMu.Lock()
	bisector.badCommit = badCommit
	bisector.goodCommits = goodCommits
	bisector.finished = finished
	bisector.stateMu.Unlock()
}

// Step executes one step of git bisect. It stores the test result and related info.
func (bisector *GitBisector) Step(testResult server.ResultType) {
	bisector.touch()
	bisector.log.WithField("testResult", testResult).Debug("Git bisect step")

	bisector.stateMu.RLock()
	alreadyFinished := bisector.finished
	bisector.stateMu.RUnlock()

	if !alreadyFinished {
		w := bisector.log.WithField("cmd", "bisectStep").Writer()
		defer w.Close()

		finished, err := bisector.repo.BisectStep(testResult, w)
		check.Panic(err, bisector.log, "Failed to perform a bisect step")

		bisector.stateMu.Lock()
		bisector.finished = finished
		bisector.stateMu.Unlock()
	}
}

// Finish checks whether bisect finishes and perform result aggregation if true.
// It fetches and aggregates test results and send bisect log as email.
func (bisector *GitBisector) Finish() bool {
	bisector.stateMu.RLock()
	finished := bisector.finished
	bisector.stateMu.RUnlock()
	if !finished {
		return false
	}

	bisector.log.Info("Git bisect finished")
	defer bisector.Clean()

	gce, err := gcp.NewService(bisector.gsBucket)
	if !check.NoError(err, bisector.log, "Failed to connect to GCE service") {
		return true
	}
	defer gce.Close()

	bisector.aggResults(gce)
	bisector.packResults(gce)
	bisector.emailReport()

	return true
}

func (bisector *GitBisector) aggResults(gce *gcp.Service) {
	bisector.log.Info("Fetching test results")
	file, err := os.Create(bisector.resultsDir + "report")
	if !check.NoError(err, bisector.log, "Failed to create file") {
		return
	}
	defer file.Close()

	fmt.Fprint(file, bisector.Info().String())

	bisector.stateMu.RLock()
	history := make([]string, len(bisector.testHistory))
	copy(history, bisector.testHistory)
	bisector.stateMu.RUnlock()

	for _, testID := range history {
		fmt.Fprintf(file, "\n============TEST %s============\n", testID)
		reportFile, err := bisector.getResults(testID, gce)
		if err == nil {
			sourceFile, err := os.Open(reportFile)
			if check.NoError(err, bisector.log, "Failed to open file") {
				_, err = io.Copy(file, sourceFile)
				check.NoError(err, bisector.log, "Failed to copy file")

				sourceFile.Close()
			}
		}
		if err != nil {
			fmt.Fprintf(file, "No test results available, check prior emails or log for errors\n")
		}
	}
}

func (bisector *GitBisector) getResults(testID string, gce *gcp.Service) (string, error) {
	if err := check.ValidateTestID(testID); err != nil {
		return "", fmt.Errorf("invalid testID: %w", err)
	}
	prefix := fmt.Sprintf("%s/results.%s-%s.", bisector.bucketSubdir, server.LTMUserName, testID)
	resultFiles, err := gce.GetFileNames(prefix)
	if !check.NoError(err, bisector.log, "Failed to get GS filenames") {
		return "", err
	}

	if len(resultFiles) >= 1 {
		bisector.log.WithField("resultURL", resultFiles[0]).Debug("Found result file url")

		url := fmt.Sprintf("gs://%s/%s", bisector.gsBucket, resultFiles[0])
		cmd := exec.Command("gce-xfstests", "get-results",
			"--unpack-dir", bisector.logDir, url)
		cmdLog := bisector.log.WithField("cmd", cmd.String())
		w := cmdLog.Writer()
		defer w.Close()
		err := check.Run(cmd, check.RootDir, check.EmptyEnv, w, w)
		if !check.NoError(err, cmdLog, "Failed to run get-results") {
			return "", err
		}

		tmpResultsDir := fmt.Sprintf("%s/results-%s-%s",
			bisector.logDir, server.LTMUserName, testID)
		unpackedResultsDir := bisector.logDir + "results/" + testID + "/"

		if check.DirExists(tmpResultsDir) {
			os.RemoveAll(unpackedResultsDir)
			err = os.Rename(tmpResultsDir, unpackedResultsDir)
			if !check.NoError(err, bisector.log, "Failed to move dir") {
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

func (bisector *GitBisector) packResults(gce *gcp.Service) {
	bisector.log.Info("Packing test results")
	aggFile := fmt.Sprintf("%sresults.%s-%s-bisector", bisector.logDir, server.LTMUserName, bisector.testID)

	cmd := exec.Command("tar", "-cf", aggFile+".tar", "-C", bisector.resultsDir, ".")
	cmdLog := bisector.log.WithField("cmd", cmd.Args)
	w1 := cmdLog.Writer()
	defer w1.Close()
	err := check.Run(cmd, check.RootDir, check.EmptyEnv, w1, w1)
	if !check.NoError(err, cmdLog, "Failed to create tarball") {
		return
	}

	cmd = exec.Command("xz", "-6ef", aggFile+".tar")
	cmdLog = bisector.log.WithField("cmd", cmd.Args)
	w2 := cmdLog.Writer()
	defer w2.Close()
	err = check.Run(cmd, check.RootDir, check.EmptyEnv, w2, w2)
	if !check.NoError(err, cmdLog, "Failed to create xz compressed tarball") {
		return
	}

	bisector.log.Info("Uploading repacked results tarball")
	gsPath := fmt.Sprintf("%s/results.%s-%s-bisector.tar.xz", bisector.bucketSubdir, server.LTMUserName, bisector.testID)
	err = gce.UploadFile(aggFile+".tar.xz", gsPath)
	if !check.NoError(err, bisector.log, "Failed to upload results tarball") {
		return
	}

	bisector.log.Info("Removing separate results tarball")
	bisector.stateMu.RLock()
	history := make([]string, len(bisector.testHistory))
	copy(history, bisector.testHistory)
	bisector.stateMu.RUnlock()
	for _, testID := range history {
		prefix := fmt.Sprintf("%s/results.%s-%s", bisector.bucketSubdir, server.LTMUserName, testID)
		_, err = gce.DeleteFiles(prefix)
		check.NoError(err, bisector.log, "Failed to delete file")
	}

	os.Remove(aggFile + ".tar.xz")
}

func (bisector *GitBisector) emailReport() {
	bisector.log.Info("Sending email report")
	subject := "xfstests bisector summary " + bisector.testID

	b, err := ioutil.ReadFile(bisector.resultsDir + "report")
	content := string(b)
	if !check.NoError(err, bisector.log, "Failed to read the report file") {
		content = "Unable to generate bisector summary report"
	}

	err = email.Send(subject, content, bisector.reportReceiver)
	check.NoError(err, bisector.log, "Failed to send the email")
}

// GetCommit returns the repo's head.
func (bisector *GitBisector) GetCommit() string {
	commit, err := bisector.repo.GetCommit(os.Stdout)
	check.Panic(err, bisector.log, "Failed to get commit")
	return commit
}

// Build builds the current commit for the bisector
// It returns a resultType other than DefaultResult to skip
// running tests and perform next bisect step immediately
func (bisector *GitBisector) Build() server.ResultType {
	bisector.touch()
	commit := bisector.GetCommit()
	bisector.log.WithField("commit", commit).Debug("Git bisect build")
	shortCommit := commit
	if len(shortCommit) > 8 {
		shortCommit = shortCommit[:8]
	}
	newTestID := bisector.testID + "-" + shortCommit

	gsPath := fmt.Sprintf("gs://%s/kernels/bzImage-%s-onerun.deb", bisector.gsBucket, newTestID)

	bisector.testRequest.Options.GsKernel = gsPath
	bisector.testRequest.Options.CommitID = commit
	bisector.testRequest.ExtraOptions.TestID = newTestID
	bisector.testRequest.ExtraOptions.Requester = server.KCSBisectStep

	bisector.stateMu.Lock()
	bisector.testHistory = append(bisector.testHistory, newTestID)
	bisector.stateMu.Unlock()

	buildLog := bisector.logDir + newTestID + ".build"
	gsConfig := bisector.testRequest.Options.KConfig
	kConfigOpts := bisector.testRequest.Options.KConfigOpts
	kbuildOpts := bisector.testRequest.Options.KbuildOpts
	arch := bisector.testRequest.Options.Arch

	if logging.MOCK {
		return MockRunBuild(bisector.repo, bisector.gsBucket, gsPath, gsConfig, kConfigOpts, kbuildOpts, arch, newTestID, buildLog, bisector.log)
	}
	err := RunBuild(bisector.repo, bisector.gsBucket, gsPath, gsConfig, kConfigOpts, kbuildOpts, arch, newTestID, buildLog)
	if !check.NoError(err, bisector.log, "Failed to build and upload kernel, skip commit") {
		return server.Error
	}
	return server.DefaultResult
}

// StartTest sends a test request to LTM
func (bisector *GitBisector) StartTest() {
	server.SendInternalRequest(bisector.testRequest, bisector.log, false)
}

// Clean removes the repo that binds to the bisector and closes log.
// It also disables the expire monitor and removes itself from bisectorMap.
func (bisector *GitBisector) Clean() {
	bisector.cleanOnce.Do(func() {
		bisector.stateMu.Lock()
		bisector.cleaned = true
		bisector.stateMu.Unlock()

		bisectorLock.Lock()
		delete(bisectorMap, bisector.testID)
		bisectorLock.Unlock()

		close(bisector.done)
		if bisector.log != nil {
			bisector.log.Debug("Git bisect clean up")
		}

		if bisector.repo != nil {
			err := bisector.repo.Delete()
			if bisector.log != nil {
				check.NoError(err, bisector.log, "Failed to clean up repo")
			}
		}

		if bisector.resultsDir != "" {
			os.RemoveAll(bisector.resultsDir)
		}
		if bisector.log != nil {
			logging.CloseLog(bisector.log)
		}
	})
}

// Exit handles bisector panic and clean up.
// It passes on the panic for error handling.
func (bisector *GitBisector) Exit() {
	if r := recover(); r != nil {
		bisector.log.Error("Bisector exits with error, clean up")
		bisector.Clean()

		panic(r)
	}
}

// Info returns structured bisector information.
func (bisector *GitBisector) Info() server.BisectorInfo {
	var result string
	if bisector.repo != nil {
		var err error
		result, err = bisector.repo.BisectLog(os.Stdout)
		if err != nil {
			result = "Bisect log not available"
		}
	} else {
		result = "Bisect log not available"
	}
	resultLines := strings.Split(result, "\n")

	bisector.stateMu.RLock()
	badCommit := bisector.badCommit
	goodCommits := make([]string, len(bisector.goodCommits))
	copy(goodCommits, bisector.goodCommits)
	lastActive := bisector.lastActive
	bisector.stateMu.RUnlock()

	var repoURL string
	if bisector.testRequest.Options != nil {
		repoURL = bisector.testRequest.Options.GitRepo
	}

	return server.BisectorInfo{
		ID:          bisector.testID,
		Command:     bisector.origCmd,
		Repo:        repoURL,
		BadCommit:   badCommit,
		GoodCommits: goodCommits,
		LastActive:  time.Since(lastActive).Round(time.Second).String(),
		Log:         resultLines,
	}
}

// CheckActive checks whether a bisector is active and clean it up when expired.
func (bisector *GitBisector) CheckActive() bool {
	bisector.stateMu.RLock()
	cleaned := bisector.cleaned
	active := bisector.activeOps > 0
	lastActive := bisector.lastActive
	bisector.stateMu.RUnlock()

	if cleaned || active || time.Since(lastActive) <= bisectorTimeout {
		return cleaned
	}

	bisector.opMu.Lock()
	defer bisector.opMu.Unlock()

	bisector.stateMu.RLock()
	cleaned = bisector.cleaned
	active = bisector.activeOps > 0
	lastActive = bisector.lastActive
	bisector.stateMu.RUnlock()

	if cleaned || active || time.Since(lastActive) <= bisectorTimeout {
		return cleaned
	}

	if bisector.log != nil {
		bisector.log.WithField(
			"lastActive", lastActive.Format(time.Stamp),
		).Warn("Bisector timeout, cleaning resources")
	}
	bisector.Clean()
	return true
}

func reserveBisectorID(testID string) bool {
	bisectorLock.Lock()
	defer bisectorLock.Unlock()
	if _, ok := bisectorMap[testID]; ok || pendingBisectors[testID] {
		return false
	}
	pendingBisectors[testID] = true
	return true
}

// HasActiveOrPendingBisectors reports whether any bisector is active or initializing.
func HasActiveOrPendingBisectors() bool {
	bisectorLock.Lock()
	defer bisectorLock.Unlock()
	return len(bisectorMap) > 0 || len(pendingBisectors) > 0
}

/*
RunBisect performs a git bisect task.

Depending on RequestType, it either initiates a new git bisect task or steps
on an existing task using the test result. If the task finishes, it sends
a report to the user and cleans up related resources.
If the current HEAD in the request differs from the bisector, it does nothing.
If the build fails, it bisect skip the current commit.
*/
func RunBisect(c server.TaskRequest, testID string, serverLog *logrus.Entry) {
	log := serverLog.WithField("testID", testID)
	if err := check.ValidateTestID(testID); err != nil {
		log.WithError(err).Error("Invalid testID in RunBisect")
		return
	}
	log.Info("Start git bisect task")

	logFile := logging.KCSLogDir + testID + "/run.log"
	subject := "xfstests KCS bisect failure " + testID
	defer email.ReportFailure(log, logFile, c.Options.ReportEmail, subject)

	var bisector *GitBisector
	var ok bool
	if c.ExtraOptions.Requester == server.LTMBisectStart {
		if !reserveBisectorID(testID) {
			log.Panic("Git bisector already exists")
		}
		registered := false
		defer func() {
			if !registered {
				bisectorLock.Lock()
				delete(pendingBisectors, testID)
				bisectorLock.Unlock()
			}
		}()

		var err error
		bisector, err = NewGitBisector(c, testID)
		check.Panic(err, log, "Failed to initiate git bisector")

		bisector.opMu.Lock()
		defer bisector.opMu.Unlock()
		if !bisector.beginOp() {
			log.Panic("Git bisector has already been cleaned up")
		}
		defer bisector.endOp()

		bisectorLock.Lock()
		delete(pendingBisectors, testID)
		bisectorMap[testID] = bisector
		registered = true
		bisectorLock.Unlock()
	} else {
		bisectorLock.Lock()
		bisector, ok = bisectorMap[testID]
		bisectorLock.Unlock()
		if !ok {
			log.Panic("Git bisector doesn't exist")
		}

		bisector.opMu.Lock()
		defer bisector.opMu.Unlock()
		if !bisector.beginOp() {
			log.Panic("Git bisector doesn't exist")
		}
		defer bisector.endOp()

		curCommit := bisector.GetCommit()
		if c.Options.CommitID != curCommit {
			log.WithFields(logrus.Fields{
				"request":  c.Options.CommitID,
				"bisector": curCommit,
			}).Panic("CommitID in request differs from bisector")
		}
	}

	defer bisector.Exit()
	if c.ExtraOptions.Requester == server.LTMBisectStart {
		bisector.Start()
	} else {
		bisector.Step(c.ExtraOptions.TestResult)
	}

	if bisector.Finish() {
		return
	}

	testResult := bisector.Build()
	for testResult != server.DefaultResult {
		bisector.Step(testResult)
		if bisector.Finish() {
			return
		}
		testResult = bisector.Build()
	}

	bisector.StartTest()
}

// BisectorStatus returns the info for active git bisectors.
func BisectorStatus() []server.BisectorInfo {
	bisectorLock.Lock()
	bisectors := make([]*GitBisector, 0, len(bisectorMap))
	for _, v := range bisectorMap {
		bisectors = append(bisectors, v)
	}
	bisectorLock.Unlock()

	infoList := make([]server.BisectorInfo, 0, len(bisectors))
	for _, v := range bisectors {
		infoList = append(infoList, v.Info())
	}
	sort.Slice(infoList, func(i, j int) bool {
		return infoList[i].ID < infoList[j].ID
	})
	return infoList
}
