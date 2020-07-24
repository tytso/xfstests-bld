package main

import (
	"fmt"
	"strings"

	"gce-server/util/check"
	"gce-server/util/email"
	"gce-server/util/gcp"
	"gce-server/util/git"
	"gce-server/util/logging"
	"gce-server/util/server"

	"github.com/sirupsen/logrus"
)

// GitBisector performs a git bisect operation on a repo branch.
// Each watcher keeps a unique repository to save the states for bisect
// progress.
type GitBisector struct {
	testID         string
	reportReceiver string
	testRequest    server.TaskRequest

	repo        *git.Repository
	badCommit   string
	goodCommits []string
	finished    bool
	log         *logrus.Entry
}

// bisectorMap indexed bisectors by testID.
// testID are guaranteed to be unique so no thread-safe protections.
var bisectorMap = make(map[string]*GitBisector)

// NewGitBisector constructs a new git bisect manager from a bisect request.
// The repo is initialized with a git bisect session.
func NewGitBisector(c server.TaskRequest, testID string) *GitBisector {
	log := server.Log.WithField("testID", testID)
	log.Info("Initiating git bisector")

	repo, err := git.NewRepository(testID, c.Options.GitRepo)
	check.Panic(err, log, "Failed to clone repo")

	badCommit := c.Options.BadCommit
	goodCommits := strings.Split(c.Options.GoodCommit, "|")

	finished, err := repo.BisectStart(badCommit, goodCommits)
	check.Panic(err, log, "Failed to start bisect")

	bisector := GitBisector{
		testID:         testID,
		reportReceiver: c.Options.ReportEmail,
		testRequest:    c,

		repo:        repo,
		badCommit:   c.Options.BadCommit,
		goodCommits: goodCommits,
		finished:    finished,
		log:         log,
	}

	return &bisector
}

// Step executed one step of git bisect.
func (bisector *GitBisector) Step(testResult server.ResultType) {
	bisector.log.Debug("Git bisect step")

	if !bisector.finished {
		finished, err := bisector.repo.BisectStep(testResult)
		check.NoError(err, bisector.log, "Failed to perform a bisect step")

		bisector.finished = finished
	}
}

// Finished indicated whether the git bisect has finished.
func (bisector *GitBisector) Finished() bool {
	return bisector.finished
}

// GetReport returns the biect log report.
func (bisector *GitBisector) GetReport() string {
	bisector.log.Debug("Git bisect get report")
	if !bisector.finished {
		bisector.log.Panic("Bisect log not available")
	}

	result, err := bisector.repo.BisectLog()
	check.Panic(err, bisector.log, "Failed to get bisect log")

	return result
}

// GetRepo returns the repo.
func (bisector *GitBisector) GetRepo() *git.Repository {
	return bisector.repo
}

// Clean removes the repo that binds to the bisector.
func (bisector *GitBisector) Clean() {
	bisector.log.Debug("Git bisect clean up")

	err := bisector.repo.Delete()
	check.NoError(err, bisector.log, "Failed to clean up bisector")
}

// RunBisect performs a git bisect task.
// Depending to TaskRequest, it either initiates a new git bisect task, or steps
// on an existing task using the test result. If the task finishes, it sends
// a report to the user and cleans up related resources.
func RunBisect(c server.TaskRequest, testID string) {
	log := server.Log.WithField("testID", testID)
	log.Info("Start git bisect task")

	bisectLog := logging.KCSLogDir + testID + ".log"
	subject := "xfstests KCS bisect failure " + testID
	defer email.ReportFailure(log, bisectLog, c.Options.ReportEmail, subject)

	var bisector *GitBisector
	var ok bool
	if c.ExtraOptions.Requester == server.LTMBisectStart {
		if _, ok = bisectorMap[testID]; ok {
			log.Panic("Git bisector already exists")
		}
		bisector = NewGitBisector(c, testID)
		bisectorMap[testID] = bisector
	} else {
		if bisector, ok = bisectorMap[testID]; !ok {
			log.Panic("Git bisector doesn't exist")
		}
		bisector.Step(c.ExtraOptions.TestResult)
	}

	if bisector.Finished() {
		log.Info("Git bisect finished, sending report")
		delete(bisectorMap, testID)
		defer bisector.Clean()

		subject := "xfstests bisect report " + testID
		err := email.Send(subject, bisector.GetReport(), bisector.reportReceiver)
		check.Panic(err, log, "Failed to send email")

	} else {

		gsBucket, err := gcp.GceConfig.Get("GS_BUCKET")
		check.Panic(err, log, "Failed to get gs bucket config")
		gsPath := fmt.Sprintf("gs://%s/kernels/bzImage-%s-onerun", gsBucket, testID)

		buildLog := logging.KCSLogDir + testID + ".build"

		if logging.MOCK {
			result := MockRunBuild(bisector.GetRepo(), gsBucket, gsPath, testID, buildLog, log)
			c.Options.GsKernel = gsPath
			c.ExtraOptions.Requester = server.KCSBisectStep
			c.ExtraOptions.TestResult = result
			server.SendInternalRequest(c, log, false)
			return
		}

		runBuild(bisector.GetRepo(), gsBucket, gsPath, testID, buildLog, log)

		c.Options.GsKernel = gsPath
		c.ExtraOptions.Requester = server.KCSBisectStep
		server.SendInternalRequest(c, log, false)
	}
}
