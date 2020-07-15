package main

import (
	"fmt"
	"strings"

	"gce-server/logging"
	"gce-server/server"
	"gce-server/util"

	"github.com/sirupsen/logrus"
)

// GitBisector performs a git bisect operation on a repo branch.
// Each watcher keeps a unique repository to save the states for bisect
// progress.
type GitBisector struct {
	testID         string
	reportReceiver string
	testRequest    server.TaskRequest

	repo        *util.Repository
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

	repo, err := util.NewRepository(testID, c.Options.GitRepo)
	logging.CheckPanic(err, log, "Failed to clone repo")

	badCommit := c.Options.BadCommit
	goodCommits := strings.Split(c.Options.GoodCommit, "|")

	finished, err := repo.BisectStart(badCommit, goodCommits)
	logging.CheckPanic(err, log, "Failed to start bisect")

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
func (bisector *GitBisector) Step(good bool) {
	bisector.log.Debug("Git bisect step")

	if !bisector.finished {
		finished, err := bisector.repo.BisectStep(good)
		logging.CheckNoError(err, bisector.log, "Failed to perform a bisect step")

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
	logging.CheckPanic(err, bisector.log, "Failed to get bisect log")

	return result
}

// GetRepo returns the repo.
func (bisector *GitBisector) GetRepo() *util.Repository {
	return bisector.repo
}

// Clean removes the repo that binds to the bisector.
func (bisector *GitBisector) Clean() {
	bisector.log.Debug("Git bisect clean up")

	err := bisector.repo.Delete()
	logging.CheckNoError(err, bisector.log, "Failed to clean up bisector")
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
	defer util.ReportFailure(log, bisectLog, c.Options.ReportEmail, subject)

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
		err := util.SendEmail(subject, bisector.GetReport(), bisector.reportReceiver)
		logging.CheckPanic(err, log, "Failed to send email")

	} else {
		config, err := util.GetConfig(util.GceConfigFile)
		logging.CheckPanic(err, log, "Failed to get config")

		gsBucket := config.Get("GS_BUCKET")
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
