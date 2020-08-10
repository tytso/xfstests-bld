package main

import (
	"testing"

	"gce-server/util/server"
)

func TestBisectSkip(t *testing.T) {
	c := server.TaskRequest{
		Options: &server.UserOptions{
			GitRepo:    "https://github.com/tytso/ext4",
			BadCommit:  "bisect-test-ext4-035",
			GoodCommit: "v5.6",
		},
	}
	bisector := NewGitBisector(c, "test")
	t.Logf(bisector.GetCommit())

	bisector.Step(server.Pass)

	t.Logf(bisector.GetCommit())
}
