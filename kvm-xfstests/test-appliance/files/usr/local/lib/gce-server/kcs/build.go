package main

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"gce-server/util"
)

var buildLock sync.Mutex

// StartBuild starts a kernel build task.
// A unique testID is generated if not specified in the request, and the
// kernel image is uploaded to gs bucket at path /kernels/bzImage-<testID>
func StartBuild(c util.UserRequest) string {
	testID := util.GetTimeStamp()
	if c.ExtraOptions != nil {
		testID = c.ExtraOptions.TestID
	}
	config := util.GetConfig(util.GceConfigFile)
	gsBucket := config.Get("GS_BUCKET")
	gsPath := fmt.Sprintf("gs://%s/kernels/bzImage-%s", gsBucket, testID)

	go runBuild(c.Options.GitRepo, c.Options.CommitID, gsBucket, gsPath)

	return gsPath
}

func runBuild(url string, commit string, gsBucket string, gsPath string) {
	buildLock.Lock()
	defer buildLock.Unlock()
	cmd := exec.Command(util.FetchBuildScript)
	env := map[string]string{
		"GIT_REPO":     url,
		"COMMIT":       commit,
		"GS_BUCKET":    gsBucket,
		"GS_PATH":      gsPath,
		"BUILD_KERNEL": "yes",
	}
	err := util.CheckRun(cmd, util.RootDir, env, os.Stdout, os.Stderr)
	util.Check(err)

}
