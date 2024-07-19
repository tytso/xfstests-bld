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
	finished    bool
	badCommit   string
	goodCommits []string
	lastActive  time.Time
	done        chan bool

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
	bisectorMap  = make(map[string]*GitBisector)
	bisectorLock sync.Mutex
)

// NewGitBisector constructs a new git bisect manager from a bisect request.
// The repo is initialized with a git bisect session.
// Creates a monitor goroutine that removes expired bisectors.
func NewGitBisector(c server.TaskRequest, testID string) *GitBisector {
	logDir := logging.KCSLogDir + testID + "/"
	err := check.CreateDir(logDir)
	if err != nil {
		panic(err)
	}

	logFile := logDir + "run.log"
	log := logging.InitLogger(logFile)
	log.Info("Initiating git bisector")

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

	w := log.WithField("cmd", "bisectInit").Writer()
	defer w.Close()

	repo, err := git.NewRepository(testID, c.Options.GitRepo, w)
	check.Panic(err, log, "Failed to clone repo")

	badCommit := c.Options.BadCommit
	goodCommits := strings.Split(c.Options.GoodCommit, "|")

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
		done:        make(chan bool),

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
				bisector.CheckActive()
			}
		}
	}()

	return &bisector
}

// Start starts the bisect.
func (bisector *GitBisector) Start() {
	bisector.lastActive = time.Now()
	bisector.log.Debug("Git bisect Start")

	w := bisector.log.WithField("cmd", "bisectStart").Writer()
	defer w.Close()

	bisector.log.Debug("Validate commits")

	valid, err := bisector.repo.Valid(bisector.badCommit, w)
	if !valid && !strings.HasPrefix(bisector.badCommit, "origin/") {
		bisector.badCommit = "origin/" + bisector.badCommit
		valid, err = bisector.repo.Valid(bisector.badCommit, w)
	}
	check.Panic(err, bisector.log, "Failed to validate badCommit")

	for i, rev := range bisector.goodCommits {
		valid, err = bisector.repo.Valid(rev, w)
		if !valid && !strings.HasPrefix(rev, "origin/") {
			bisector.goodCommits[i] = "origin/" + rev
			valid, err = bisector.repo.Valid(bisector.goodCommits[i], w)
		}
		check.Panic(err, bisector.log, "Failed to validate goodCommit")
	}

	finished, err := bisector.repo.BisectStart(bisector.badCommit, bisector.goodCommits, w)
	check.Panic(err, bisector.log, "Failed to start bisect")

	bisector.finished = finished
}

// Step executes one step of git bisect. It stores the test result and related info.
func (bisector *GitBisector) Step(testResult server.ResultType) {
	bisector.lastActive = time.Now()
	bisector.log.WithField("testResult", testResult).Debug("Git bisect step")

	if !bisector.finished {
		w := bisector.log.WithField("cmd", "bisectStep").Writer()
		defer w.Close()

		finished, err := bisector.repo.BisectStep(testResult, w)
		check.Panic(err, bisector.log, "Failed to perform a bisect step")

		bisector.finished = finished
	}
}

// Finish checks whether bisect finishes and perform result aggregation if true.
// It fetches and aggregates test results and send bisect log as email.
func (bisector *GitBisector) Finish() bool {
	if !bisector.finished {
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

	for _, testID := range bisector.testHistory {
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
	for _, testID := range bisector.testHistory {
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
	bisector.lastActive = time.Now()
	commit := bisector.GetCommit()
	bisector.log.WithField("commit", commit).Debug("Git bisect build")
	newTestID := bisector.testID + "-" + commit[:8]

	gsPath := fmt.Sprintf("gs://%s/kernels/bzImage-%s-onerun.deb", bisector.gsBucket, newTestID)

	bisector.testRequest.Options.GsKernel = gsPath
	bisector.testRequest.Options.CommitID = commit
	bisector.testRequest.ExtraOptions.TestID = newTestID
	bisector.testRequest.ExtraOptions.Requester = server.KCSBisectStep

	bisector.testHistory = append(bisector.testHistory, newTestID)

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
	bisectorLock.Lock()
	defer bisectorLock.Unlock()
	bisector.log.Debug("Git bisect clean up")

	err := bisector.repo.Delete()
	check.NoError(err, bisector.log, "Failed to clean up repo")

	delete(bisectorMap, bisector.testID)
	os.RemoveAll(bisector.resultsDir)
	logging.CloseLog(bisector.log)
	bisector.done <- true
	close(bisector.done)
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
	result, err := bisector.repo.BisectLog(os.Stdout)
	if err != nil {
		result = "Bisect log not available"
	}
	resultLines := strings.Split(result, "\n")
	return server.BisectorInfo{
		ID:          bisector.testID,
		Command:     bisector.origCmd,
		Repo:        bisector.testRequest.Options.GitRepo,
		BadCommit:   bisector.badCommit,
		GoodCommits: bisector.goodCommits,
		LastActive:  time.Since(bisector.lastActive).Round(time.Second).String(),
		Log:         resultLines,
	}
}

// CheckActive checks whether a bisector is active and clean it up when expired.
func (bisector *GitBisector) CheckActive() {
	if time.Since(bisector.lastActive) > bisectorTimeout {
		bisector.log.WithField(
			"lastActive", bisector.lastActive.Format(time.Stamp),
		).Warn("Bisector timeout, cleaning resources")
		bisector.Clean()
	}
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
	log.Info("Start git bisect task")

	logFile := logging.KCSLogDir + testID + "/run.log"
	subject := "xfstests KCS bisect failure " + testID
	defer email.ReportFailure(log, logFile, c.Options.ReportEmail, subject)

	var bisector *GitBisector
	var ok bool
	if c.ExtraOptions.Requester == server.LTMBisectStart {
		if _, ok = bisectorMap[testID]; ok {
			log.Panic("Git bisector already exists")
		}

		bisector = NewGitBisector(c, testID)

		bisectorLock.Lock()
		bisectorMap[testID] = bisector
		bisectorLock.Unlock()
	} else {
		bisectorLock.Lock()
		bisector, ok = bisectorMap[testID]
		bisectorLock.Unlock()
		if !ok {
			log.Panic("Git bisector doesn't exist")
		}

		if c.Options.CommitID != bisector.GetCommit() {
			log.WithFields(logrus.Fields{
				"request":  c.Options.CommitID,
				"bisector": bisector.GetCommit(),
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
	defer bisectorLock.Unlock()
	infoList := []server.BisectorInfo{}
	for _, v := range bisectorMap {
		infoList = append(infoList, v.Info())
	}
	sort.Slice(infoList, func(i, j int) bool {
		return infoList[i].ID < infoList[j].ID
	})
	return infoList
}
