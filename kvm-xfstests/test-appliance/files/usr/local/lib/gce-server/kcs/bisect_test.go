package main

import (
	"os"
	"testing"

	"gce-server/util/server"
)

func TestBisect(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Error(err)
	}
	if hostname != "xfstests-kcs" {
		t.Skip("test only runs on KCS server")
	}

	c := server.TaskRequest{
		Options: &server.UserOptions{
			GitRepo:    "https://github.com/tytso/ext4.git",
			BadCommit:  "bisect-test-ext4-035",
			GoodCommit: "v5.6",
		},
	}
	bisector := NewGitBisector(c, "test")
	bisector.Start()
	if bisector.GetCommit() != "c870e04e71136d57817526add31b6abe2b451c63" {
		t.Error("bisector commit mismatch")
	}

	bisector.Step(server.Pass)
	if bisector.GetCommit() != "f303861332f011ba3624040518110f6d20d4fa93" {
		t.Error("bisector commit mismatch")
	}

	bisector.Step(server.Fail)
	if bisector.GetCommit() != "bd8fcbd34439e72648ccdc74987fcff372688d88" {
		t.Error("bisector commit mismatch")
	}

	bisector.Clean()
}
