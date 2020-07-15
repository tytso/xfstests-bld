package main

import (
	"gce-server/logging"
	"gce-server/server"
	"gce-server/util"
)

// MockStartBuild fetches a mock kernel repo and reads the mock kernel
// from mock.txt. It then sends a mock build request back to LTM.
func MockStartBuild(c server.TaskRequest, testID string) {
	log := server.Log.WithField("testID", testID)
	log.Info("Start building mock kernel")

	repo, err := util.NewSimpleRepository(c.Options.GitRepo, c.Options.CommitID)
	logging.CheckPanic(err, log, "Failed to clone repo")

	c.ExtraOptions.Requester = "test"

	lines, err := util.ReadLines(repo.Dir() + "mock.txt")
	switch lines[0] {
	case "good":
		fallthrough
	case "bad":
		fallthrough
	case "undefined":
		c.ExtraOptions.MockState = lines[0]
	default:
		panic("mock.txt in wrong format")
	}

	sendRequest(c, log)
}
