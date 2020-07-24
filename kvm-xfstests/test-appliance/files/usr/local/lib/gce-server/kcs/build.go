package main

import (
	"fmt"
	"os"
	"sync"

	"gce-server/util/check"
	"gce-server/util/email"
	"gce-server/util/gcp"
	"gce-server/util/git"
	"gce-server/util/logging"
	"gce-server/util/server"
)

// repoMap indexes repos by repo url.
// repoLock protects map access and ensures one build at a time
var (
	repoMap  = make(map[string]*git.Repository)
	repoLock sync.Mutex
)

func init() {
	err := check.CreateDir(logging.KCSLogDir)
	if err != nil {
		panic("Failed to create dir")
	}
}

// StartBuild starts a kernel build task.
// The kernel image is uploaded to gs bucket at path /kernels/bzImage-<testID>.
// If ExtraOptions is not nil, it rewrites gsKernel in original request and
// send it back to LTM to init a test.
func StartBuild(c server.TaskRequest, testID string) {
	log := server.Log.WithField("testID", testID)
	log.Info("Start building kernel")

	buildLog := logging.KCSLogDir + testID + ".build"
	subject := "xfstests KCS build failure " + testID
	defer email.ReportFailure(log, buildLog, c.Options.ReportEmail, subject)

	gsBucket, err := gcp.GceConfig.Get("GS_BUCKET")
	check.Panic(err, log, "Failed to get gs bucket config")
	gsPath := fmt.Sprintf("gs://%s/kernels/bzImage-%s-onerun", gsBucket, testID)

	id, err := git.ParseURL(c.Options.GitRepo)
	check.Panic(err, log, "Failed to parse repo url")

	repoLock.Lock()
	defer repoLock.Unlock()

	repo, ok := repoMap[id]

	cmdLog := log.WithField("repoId", id)
	w := cmdLog.WithField("cmd", "newRepo").Writer()

	if !ok {
		cmdLog.Debug("Cloning repo")
		repo, err = git.NewRepository(id, c.Options.GitRepo, w)
		check.Panic(err, cmdLog, "Failed to clone repo")

		repoMap[id] = repo
	} else {
		cmdLog.Debug("Existing repo found")
	}

	err = repo.Checkout(c.Options.CommitID, w)
	check.Panic(err, cmdLog, "Failed to checkout to commit")

	if logging.MOCK {
		result := MockRunBuild(repo, gsBucket, gsPath, testID, buildLog, log)
		c.Options.GsKernel = gsPath
		c.ExtraOptions.Requester = server.KCSTest
		c.ExtraOptions.TestResult = result
		server.SendInternalRequest(c, log, false)
		return
	}

	err = runBuild(repo, gsBucket, gsPath, testID, buildLog)
	check.Panic(err, log, "Failed to build and upload kernel")

	if c.ExtraOptions != nil {
		c.Options.GsKernel = gsPath
		c.ExtraOptions.Requester = server.KCSTest
		server.SendInternalRequest(c, log, false)
	}
}

// runBuild builds the kernel and upload the kernel image
func runBuild(repo *git.Repository, gsBucket string, gsPath string, testID string, buildLog string) error {
	file, err := os.Create(buildLog)
	if err != nil {
		return err
	}
	defer file.Close()

	err = repo.BuildUpload(gsBucket, gsPath, file)
	file.Sync()

	return err
}
