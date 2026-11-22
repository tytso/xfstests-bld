package main

import (
	"fmt"
	"io"
	"sync"
	"testing"

	"thunk.org/gce-server/util/check"
	"thunk.org/gce-server/util/email"
	"thunk.org/gce-server/util/gcp"
	"thunk.org/gce-server/util/git"
	"thunk.org/gce-server/util/logging"
	"thunk.org/gce-server/util/server"

	"github.com/sirupsen/logrus"
)

// repoMap indexes repos by parsed repo id.
// repoLock protects map access, while repoBuildLocks serializes checkout+build
// per repository without blocking unrelated repos or LTM callback retries.
var (
	repoMap        = make(map[string]*git.Repository)
	repoBuildLocks = make(map[string]*sync.Mutex)
	repoLock       sync.Mutex
)

func getRepoBuildLock(id string) *sync.Mutex {
	repoLock.Lock()
	defer repoLock.Unlock()
	mu, ok := repoBuildLocks[id]
	if !ok {
		mu = &sync.Mutex{}
		repoBuildLocks[id] = mu
	}
	return mu
}

func getOrCloneBuildRepo(id string, repoURL string, cmdLog *logrus.Entry, w *io.PipeWriter) *git.Repository {
	repoLock.Lock()
	repo, ok := repoMap[id]
	if ok && !check.DirExists(repo.Dir()) {
		cmdLog.Warn("Cached repo directory missing, re-cloning")
		delete(repoMap, id)
		repoLock.Unlock()
		_ = repo.Delete()
		ok = false
	} else {
		repoLock.Unlock()
	}

	if !ok {
		cmdLog.Debug("Cloning repo")
		var err error
		repo, err = git.NewRepository(id, repoURL, w)
		check.Panic(err, cmdLog, "Failed to clone repo")

		repoLock.Lock()
		repoMap[id] = repo
		repoLock.Unlock()
	} else {
		cmdLog.Debug("Existing repo found")
	}
	return repo
}

func init() {
	err := check.CreateDir(logging.KCSLogDir)
	if err != nil && !testing.Testing() {
		panic("Failed to create dir")
	}
}

// StartBuild starts a kernel build task.
// The kernel image is uploaded to gs bucket at path /kernels/bzImage-<testID>.
// If ExtraOptions is not nil, it rewrites gsKernel in original request and
// send it back to LTM to init a test.
func StartBuild(c server.TaskRequest, testID string, serverLog *logrus.Entry) {
	log := serverLog.WithField("testID", testID)
	if err := check.ValidateTestID(testID); err != nil {
		log.WithError(err).Error("Invalid testID in StartBuild")
		return
	}
	if c.Options != nil && c.Options.KConfig != "" {
		if err := check.ValidateGSURI(c.Options.KConfig); err != nil {
			log.WithError(err).Error("Invalid kconfig URI in StartBuild")
			return
		}
	}
	if c.Options != nil {
		if c.Options.Arch != "" {
			if err := check.ValidateArch(c.Options.Arch); err != nil {
				log.WithError(err).Error("Invalid arch in StartBuild")
				return
			}
		}
		if c.Options.KConfigOpts != "" {
			if err := check.ValidateBuildOpts(c.Options.KConfigOpts); err != nil {
				log.WithError(err).Error("Invalid kconfig_opts in StartBuild")
				return
			}
		}
		if c.Options.KbuildOpts != "" {
			if err := check.ValidateBuildOpts(c.Options.KbuildOpts); err != nil {
				log.WithError(err).Error("Invalid kbuild_opts in StartBuild")
				return
			}
		}
		if c.Options.ReportFailEmail != "" {
			if err := check.ValidateEmailList(c.Options.ReportFailEmail); err != nil {
				log.WithError(err).Error("Invalid report_fail_email in StartBuild")
				return
			}
		}
	}
	log.Info("Start building kernel")

	buildLog := logging.KCSLogDir + testID + ".build"
	subject := "xfstests KCS build failure " + testID
	defer email.ReportFailure(log, buildLog, c.Options.ReportFailEmail, subject)

	gsBucket, err := gcp.GceConfig().Get("GS_BUCKET")
	check.Panic(err, log, "Failed to get gs bucket config")
	gsPath := fmt.Sprintf("gs://%s/kernels/bzImage-%s-onerun.deb", gsBucket, testID)

	id, err := git.ParseURL(c.Options.GitRepo)
	check.Panic(err, log, "Failed to parse repo url")

	result, isMock := checkoutAndBuild(c, id, gsBucket, gsPath, testID, buildLog, log)

	if c.ExtraOptions != nil {
		c.Options.GsKernel = gsPath
		c.ExtraOptions.Requester = server.KCSTest
		if isMock {
			c.ExtraOptions.TestResult = result
		}
		server.SendInternalRequest(c, log, false)
	}
}

func checkoutAndBuild(c server.TaskRequest, id string, gsBucket string, gsPath string, testID string, buildLog string, log *logrus.Entry) (server.ResultType, bool) {
	perRepoMu := getRepoBuildLock(id)
	perRepoMu.Lock()
	defer perRepoMu.Unlock()

	cmdLog := log.WithField("repoId", id)
	w := cmdLog.WithField("cmd", "newRepo").Writer()
	defer w.Close()

	repo := getOrCloneBuildRepo(id, c.Options.GitRepo, cmdLog, w)

	err := repo.Checkout(c.Options.CommitID, w)
	check.Panic(err, cmdLog, "Failed to checkout to commit")

	gsConfig := c.Options.KConfig
	kConfigOpts := c.Options.KConfigOpts
	kbuildOpts := c.Options.KbuildOpts
	arch := c.Options.Arch

	if logging.MOCK {
		result := MockRunBuild(repo, gsBucket, gsPath, gsConfig, kConfigOpts, kbuildOpts, arch, testID, buildLog, log)
		return result, true
	}
	cmdLog.WithField("commit", c.Options.CommitID).Info("Building kernel")

	err = RunBuild(repo, gsBucket, gsPath, gsConfig, kConfigOpts, kbuildOpts, arch, testID, buildLog)
	check.Panic(err, log, "Failed to build and upload kernel")
	log.WithField("gsPath", gsPath).Info("Kernel build and upload finished")
	return server.DefaultResult, false
}
