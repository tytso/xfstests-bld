package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestNewInitializesHTTPServerAndConcurrentShutdown(t *testing.T) {
	// Zero-value Instance guard must not panic when httpServer is nil.
	var zero Instance
	zero.Shutdown()
	zero.Close()

	inst, err := New("127.0.0.1:0", "test")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer inst.Close()

	if inst.httpServer == nil {
		t.Fatal("expected New() to initialize httpServer")
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inst.Shutdown()
		}()
	}
	wg.Wait()
}

func TestIsAlreadyExistsOutput(t *testing.T) {
	kcsOut := "Removing old cookies...\nStarting KCS instance...\nThe KCS instance already exists!\n"
	if !isAlreadyExistsOutput(kcsOut, "The KCS instance already exists!") {
		t.Fatal("expected multi-line launch-kcs output to match already-exists check")
	}

	ltmOut := "Checking config...\nThe LTM instance already exists!\nDone.\n"
	if !isAlreadyExistsOutput(ltmOut, "The LTM instance already exists!") {
		t.Fatal("expected multi-line launch-ltm output to match already-exists check")
	}

	if isAlreadyExistsOutput("ERROR: gcloud compute instances create failed\n", "The KCS instance already exists!") {
		t.Fatal("expected unrelated failure output not to match already-exists check")
	}
}

func postTaskRequestForTest(t *testing.T, reqBody TaskRequest, internal bool) (*httptest.ResponseRecorder, TaskRequest, error) {
	t.Helper()
	b, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/gce-xfstests", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	parsed, err := ParseTaskRequest(rec, req, internal)
	return rec, parsed, err
}

func TestParseTaskRequestGitRepoValidation(t *testing.T) {
	rec, _, err := postTaskRequestForTest(t, TaskRequest{
		Options: &UserOptions{
			GitRepo: "https://github.com/tytso/ext4.git",
		},
	}, false)
	if err != nil {
		t.Fatalf("expected valid git_repo to succeed, got err=%v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	for _, bad := range []string{
		"file:///etc/passwd",
		"file://localhost/tmp/repo.git",
		"--upload-pack=/bin/sh",
		"ext::sh -c evil",
		"https://github.com",
		"https://github.com/tytso/../ext4.git",
	} {
		rec, _, err := postTaskRequestForTest(t, TaskRequest{
			Options: &UserOptions{
				GitRepo: bad,
			},
		}, false)
		if err == nil {
			t.Fatalf("expected ParseTaskRequest to reject git_repo=%q", bad)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for git_repo=%q, got %d", bad, rec.Code)
		}
	}
}

func TestParseTaskRequestGitRefAndBranchValidation(t *testing.T) {
	rec, _, err := postTaskRequestForTest(t, TaskRequest{
		Options: &UserOptions{
			CommitID:   "HEAD~1",
			BranchName: "ext4/for-next",
			BadCommit:  "v6.12-rc1",
			GoodCommit: "v6.10|v6.11",
		},
	}, false)
	if err != nil || rec.Code != http.StatusOK {
		t.Fatalf("expected valid git refs to succeed, got code=%d err=%v", rec.Code, err)
	}

	badCases := []UserOptions{
		{CommitID: "--detach"},
		{CommitID: "HEAD..origin/main"},
		{BranchName: "--upload-pack=/bin/sh"},
		{BranchName: "HEAD~1"},
		{BranchName: "ext4/*"},
		{BadCommit: "--bad"},
		{GoodCommit: "v5.6||v5.7"},
		{GoodCommit: "v5.6|--good"},
		{GoodCommit: "|v5.6"},
	}
	for _, opts := range badCases {
		optsCopy := opts
		rec, _, err := postTaskRequestForTest(t, TaskRequest{Options: &optsCopy}, false)
		if err == nil || rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for %+v, got code=%d err=%v", opts, rec.Code, err)
		}
	}
}
