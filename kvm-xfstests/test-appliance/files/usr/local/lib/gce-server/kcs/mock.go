package main

import (
	"gce-server/util/check"
	"gce-server/util/git"
	"gce-server/util/server"

	"github.com/sirupsen/logrus"
)

// MockRunBuild runs a mock build. It reads mock.txt from repo to mock the test result.
func MockRunBuild(repo *git.Repository, gsBucket string, gsPath string, testID string, buildLog string, log *logrus.Entry) server.ResultType {
	log.Info("Start building mock kernel")

	lines, _ := check.ReadLines(repo.Dir() + "mock.txt")
	var result server.ResultType
	switch lines[0] {
	case "good":
		result = server.Pass
	case "bad":
		result = server.Failure
	case "undefined":
		result = server.UnknownResult
	default:
		log.Panic("mock.txt in wrong format")
	}
	return result
}
