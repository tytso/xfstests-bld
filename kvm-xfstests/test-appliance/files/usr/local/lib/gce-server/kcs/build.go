package main

import (
	"os"
	"os/exec"

	"example.com/gce-server/util"
)

func BuildKernel(c util.UserRequest) util.MsgResponse {
	go runBuild(c.Options.GitRepo, c.Options.CommitID)
	respond := util.MsgResponse{
		Status: true,
		Msg:    "started",
	}
	return respond
}

func runBuild(url string, commit string) {
	cmd := exec.Command(util.FetchBuildScript)
	config := util.GetConfig()
	env := map[string]string{
		"GIT_REPO":     url,
		"COMMIT":       commit,
		"GS_BUCKET":    config.Get("GS_BUCKET"),
		"BUILD_KERNEL": "yes",
	}
	util.CheckRun(cmd, util.RootDir, env, os.Stdout, os.Stderr)
}
