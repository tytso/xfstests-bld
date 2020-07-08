package main

import (
	"os/exec"

	"example.com/gce-server/util"
)

func buildKernel(c LTMRequest) LTMRespond {
	go runBuild(c.Options.GitRepo, c.Options.CommitID)
	respond := LTMRespond{true, "started"}
	return respond
}

func runBuild(url string, commit string) {
	cmd := exec.Command(util.FetchBuildScript)
	env := map[string]string{
		"GIT_REPO":     url,
		"COMMIT":       commit,
		"GS_BUCKET":    util.GetConfig()["GS_BUCKET"],
		"BUILD_KERNEL": "yes",
	}
	util.CheckRun(cmd, util.Rootdir, env)
}
