package main

import (
	"gce-server/util"

	"github.com/sirupsen/logrus"
)

// MockRunBuild runs a mock build. It reads mock.txt from repo to mock the test result.
func MockRunBuild(repo *util.Repository, gsBucket string, gsPath string, testID string, buildLog string, log *logrus.Entry) bool {
	log.Info("Start building mock kernel")

	lines, _ := util.ReadLines(repo.Dir() + "mock.txt")
	var result bool
	switch lines[0] {
	case "good":
		result = true
	case "bad":
		result = false
	case "undefined":
		fallthrough
	default:
		log.Panic("mock.txt in wrong format")
	}
	return result
}
