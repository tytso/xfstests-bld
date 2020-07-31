package main

import (
	"fmt"
	"os"
	"strings"
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

// GitBisector performs a git bisect operation on a repo branch.
// Each bisector keeps a unique repository to save the states for bisect
// progress.
type GitBisector struct {
	testID         string
	reportReceiver string
	testRequest    server.TaskRequest
	gsBucket       string

	repo       *git.Repository
	finished   bool
	lastActive time.Time
	log        *logrus.Entry
}

// BisectorInfo exports bisector info.
type BisectorInfo struct {
	ID          string `json:"id"`
	Repo        string `json:"repo"`
	BadCommit   string `json:"bad_commit"`
	GoodCommits string `json:"good_commits"`
	Progress    string `json:"progress"`
	LastActive  string `json:"last_active"`
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

// init initiates a checker that removes expired bisectors periodically.
func init() {
	go func() {
		ticker := time.NewTicker(checkInterval)
		for range ticker.C {
			bisectorLock.Lock()
			for _, bisector := range bisectorMap {
				bisector.CheckActive(bisectorTimeout)
			}
			bisectorLock.Unlock()
		}
	}()
}

// NewGitBisector constructs a new git bisect manager from a bisect request.
// The repo is initialized with a git bisect session.
func NewGitBisector(c server.TaskRequest, testID string, logFile string) *GitBisector {
	log := logging.InitLogger(logFile)
	log.Info("Initiating git bisector")

	w := log.WithField("cmd", "bisectStart").Writer()
	defer w.Close()

	repo, err := git.NewRepository(testID, c.Options.GitRepo, w)
	check.Panic(err, log, "Failed to clone repo")

	badCommit := c.Options.BadCommit
	goodCommits := strings.Split(c.Options.GoodCommit, "|")

	finished, err := repo.BisectStart(badCommit, goodCommits, w)
	check.Panic(err, log, "Failed to start bisect")

	gsBucket, err := gcp.GceConfig.Get("GS_BUCKET")
	check.Panic(err, log, "Failed to get gs bucket config")

	bisector := GitBisector{
		testID:         testID,
		reportReceiver: c.Options.ReportEmail,
		testRequest:    c,
		gsBucket:       gsBucket,

		repo:       repo,
		finished:   finished,
		lastActive: time.Now(),
		log:        log,
	}

	return &bisector
}

// Step executes one step of git bisect.
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

// Finish sends the bisect log to user and returns true if finished.
func (bisector *GitBisector) Finish() bool {
	if bisector.finished {
		bisector.log.Info("Git bisect finished, sending report")
		defer bisector.Clean()

		subject := "xfstests bisect report " + bisector.testID
		err := email.Send(subject, bisector.GetReport(), bisector.reportReceiver)
		check.Panic(err, bisector.log, "Failed to send email")

		return true
	}

	return false
}

// GetReport returns the biect log report.
// It returns a message instead of panic when gets error.
func (bisector *GitBisector) GetReport() string {
	bisector.log.Debug("Git bisect get report")
	if !bisector.finished {
		bisector.log.Warn("Bisector has not finished, the report is incomplete")
	}

	result, err := bisector.repo.BisectLog(os.Stdout)
	if !check.NoError(err, bisector.log, "Failed to get bisect log") {
		return "Bisect log not available"
	}
	bisector.log.Info(result)
	return result
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

	gsPath := fmt.Sprintf("gs://%s/kernels/bzImage-%s-onerun", bisector.gsBucket, newTestID)

	bisector.testRequest.Options.GsKernel = gsPath
	bisector.testRequest.Options.CommitID = commit
	bisector.testRequest.ExtraOptions.TestID = newTestID
	bisector.testRequest.ExtraOptions.Requester = server.KCSBisectStep

	buildLog := logging.KCSLogDir + newTestID + ".build"

	if logging.MOCK {
		return MockRunBuild(bisector.repo, bisector.gsBucket, gsPath, newTestID, buildLog, bisector.log)
	}

	err := RunBuild(bisector.repo, bisector.gsBucket, gsPath, newTestID, buildLog)
	if !check.NoError(err, bisector.log, "Failed to build and upload kernel, skip commit") {
		return server.UnknownResult
	}
	return server.DefaultResult
}

// StartTest sends a test request to LTM
func (bisector *GitBisector) StartTest() {
	server.SendInternalRequest(bisector.testRequest, bisector.log, false)
}

// Clean removes the repo that binds to the bisector and closes log.
// It also removes itself from bisectorMap if found.
func (bisector *GitBisector) Clean() {
	bisector.log.Debug("Git bisect clean up")

	err := bisector.repo.Delete()
	check.NoError(err, bisector.log, "Failed to clean up bisector")

	delete(bisectorMap, bisector.testID)

	logging.CloseLog(bisector.log)
}

// Exit handles bisector panic and clean up.
// It passes on the panic for error handling.
func (bisector *GitBisector) Exit() {
	if r := recover(); r != nil {
		bisector.log.Error("Bisector exits with error, clean up")
		bisector.GetReport()
		bisector.Clean()

		panic(r)
	}
}

// Info returns structured bisector information.
func (bisector *GitBisector) Info() BisectorInfo {
	result, err := bisector.repo.BisectLog(os.Stdout)
	if err != nil {
		result = "unknown"
	}
	return BisectorInfo{
		ID:          bisector.testID,
		Repo:        bisector.testRequest.Options.GitRepo,
		BadCommit:   bisector.testRequest.Options.BadCommit,
		GoodCommits: bisector.testRequest.Options.GoodCommit,
		Progress:    result,
		LastActive:  bisector.lastActive.Format(time.Stamp),
	}
}

// CheckActive checks whether a bisector is active and clean it up when expired.
func (bisector *GitBisector) CheckActive(timeout time.Duration) {
	if time.Since(bisector.lastActive) > timeout {
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
	bisectorLock.Lock()
	defer bisectorLock.Unlock()
	log := serverLog.WithField("testID", testID)
	log.Info("Start git bisect task")

	logFile := logging.KCSLogDir + testID + ".log"
	subject := "xfstests KCS bisect failure " + testID
	defer email.ReportFailure(log, logFile, c.Options.ReportEmail, subject)

	var bisector *GitBisector
	var ok bool
	if c.ExtraOptions.Requester == server.LTMBisectStart {
		if _, ok = bisectorMap[testID]; ok {
			log.Panic("Git bisector already exists")
		}

		bisector = NewGitBisector(c, testID, logFile)
		bisectorMap[testID] = bisector

		defer bisector.Exit()
	} else {
		bisector, ok = bisectorMap[testID]
		if !ok {
			log.Panic("Git bisector doesn't exist")
		}

		defer bisector.Exit()

		if c.Options.CommitID != bisector.GetCommit() {
			log.WithFields(logrus.Fields{
				"request":  c.Options.CommitID,
				"bisector": bisector.GetCommit(),
			}).Panic("CommitID in request differs from bisector")
		}
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
func BisectorStatus() []BisectorInfo {
	bisectorLock.Lock()
	defer bisectorLock.Unlock()
	infoList := []BisectorInfo{}
	for _, v := range bisectorMap {
		infoList = append(infoList, v.Info())
	}

	return infoList
}
