package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"sync"

	"gce-server/logging"
	"gce-server/server"
	"gce-server/util"

	"github.com/sirupsen/logrus"
)

var buildLock sync.Mutex

// StartBuild starts a kernel build task.
// A unique testID is generated if not specified in the request, and the
// kernel image is uploaded to gs bucket at path /kernels/bzImage-<testID>
func StartBuild(c server.UserRequest) (string, error) {
	testID := util.GetTimeStamp()
	if c.ExtraOptions != nil {
		testID = c.ExtraOptions.TestID
	}
	config, err := util.GetConfig(util.GceConfigFile)
	if err != nil {
		return "", err
	}
	gsBucket := config.Get("GS_BUCKET")
	gsPath := fmt.Sprintf("gs://%s/kernels/bzImage-%s", gsBucket, testID)
	go runBuild(c.Options.GitRepo, c.Options.CommitID, gsBucket, gsPath, testID, c.Options.ReportEmail)

	return gsPath, nil
}

func runBuild(url string, commit string, gsBucket string, gsPath string, testID string, email string) {
	buildLock.Lock()
	defer buildLock.Unlock()
	buildLog := logging.KCSLogDir + testID + ".log"

	log := server.Log.WithField("testID", testID)
	defer reportFailure(log, buildLog, testID, email)

	err := util.CreateDir(logging.KCSLogDir)
	logging.CheckPanic(err, log, "Failed to create dir")

	file, err := os.Create(buildLog)
	logging.CheckPanic(err, log, "Failed to create file")
	defer file.Close()

	cmd := exec.Command(util.FetchBuildScript)
	cmdLog := log.WithFields(logrus.Fields{
		"GitRepo":  url,
		"commitID": commit,
		"gsBucket": gsBucket,
		"gsPath":   gsPath,
		"cmd":      cmd.String(),
	})
	cmdLog.Info("Start building kernel")

	env := map[string]string{
		"GIT_REPO":     url,
		"COMMIT":       commit,
		"GS_BUCKET":    gsBucket,
		"GS_PATH":      gsPath,
		"BUILD_KERNEL": "yes",
	}

	err = util.CheckRun(cmd, util.RootDir, env, file, file)
	file.Sync()
	logging.CheckPanic(err, cmdLog, "Failed to build kernel")
}

func reportFailure(log *logrus.Entry, buildLog string, testID string, email string) {
	if r := recover(); r != nil {
		log.WithField("panic", r).Error("Build failed, sending failure report")

		subject := "xfstests KCS build failure " + testID

		msg := "unknown panic"
		switch s := r.(type) {
		case string:
			msg = s
		case error:
			msg = s.Error()
		case *logrus.Entry:
			msg = s.Message
		}

		if util.FileExists(buildLog) {
			content, err := ioutil.ReadFile(buildLog)
			if logging.CheckNoError(err, log, "Failed to read build log file") {
				msg = msg + "\n\n" + string(content)
			}
		}
		err := util.SendEmail(subject, msg, email)
		logging.CheckNoError(err, log, "Failed to send the email")
	}
}
