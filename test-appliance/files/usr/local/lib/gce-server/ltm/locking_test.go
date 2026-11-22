package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"thunk.org/gce-server/util/git"
	"thunk.org/gce-server/util/logging"
	"thunk.org/gce-server/util/server"
)

func runLocalGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func initLocalOriginRepo(t *testing.T) (string, string, string) {
	t.Helper()
	workDir := filepath.Join(t.TempDir(), "origin", "linux.git")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	runLocalGit(t, workDir, "init")
	runLocalGit(t, workDir, "config", "user.name", "Test User")
	runLocalGit(t, workDir, "config", "user.email", "test@example.com")
	runLocalGit(t, workDir, "checkout", "-B", "main")
	commit := commitToOriginRepo(t, workDir, 1)
	return "file://localhost" + workDir, workDir, commit
}

func commitToOriginRepo(t *testing.T, workDir string, n int) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workDir, "version.txt"), []byte(fmt.Sprintf("v%d\n", n)), 0644); err != nil {
		t.Fatal(err)
	}
	runLocalGit(t, workDir, "add", "version.txt")
	runLocalGit(t, workDir, "-c", "commit.gpgsign=false", "commit", "-m", fmt.Sprintf("commit %d", n))
	return runLocalGit(t, workDir, "rev-parse", "HEAD")
}

func resetWatcherGlobals(t *testing.T) {
	t.Helper()
	watcherLock.Lock()
	origMap := watcherMap
	origPending := pendingWatchers
	origForward := forwardKCSFunc
	watcherMap = make(map[string]*GitWatcher)
	pendingWatchers = make(map[string]bool)
	watcherLock.Unlock()

	t.Cleanup(func() {
		watcherLock.Lock()
		watcherMap = origMap
		pendingWatchers = origPending
		forwardKCSFunc = origForward
		watcherLock.Unlock()
	})
}

func resetSharderGlobals(t *testing.T) {
	t.Helper()
	sharderLock.Lock()
	origMap := sharderMap
	origPending := pendingSharders
	sharderMap = make(map[string]*ShardScheduler)
	pendingSharders = make(map[string]bool)
	sharderLock.Unlock()

	t.Cleanup(func() {
		sharderLock.Lock()
		sharderMap = origMap
		pendingSharders = origPending
		sharderLock.Unlock()
	})
}

func newTestGitWatcher(t *testing.T, id string, repoURL string, remote *git.RemoteRepository) *GitWatcher {
	t.Helper()
	dir := t.TempDir()
	resultsDir := filepath.Join(dir, "results") + "/"
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(dir, "run.log")
	log := logging.InitLogger(logFile)
	return &GitWatcher{
		testID:  id,
		origCmd: "gce-xfstests -c ext4/4k smoke",
		testRequest: server.TaskRequest{
			CmdLine: "Z2NlLXhmc3Rlc3RzIC1jIGV4dDQvNGsgc21va2U=",
			Options: &server.UserOptions{
				GitRepo:    repoURL,
				BranchName: "main",
			},
			ExtraOptions: &server.InternalOptions{
				TestID:    id,
				Requester: server.LTMBuild,
			},
		},
		testHistory: []server.TestInfo{},
		packHistory: []string{"pack-initial"},
		repo:        remote,
		done:        make(chan struct{}),
		logDir:      dir + "/",
		resultsDir:  resultsDir,
		logFile:     logFile,
		log:         log,
	}
}

func TestStopWatcherConcurrentAndCleanIdempotent(t *testing.T) {
	resetWatcherGlobals(t)

	repoURL, _, _ := initLocalOriginRepo(t)
	remote, err := git.NewRemoteRepository(repoURL, "main")
	if err != nil {
		t.Fatalf("NewRemoteRepository failed: %v", err)
	}
	watcher := newTestGitWatcher(t, "watch-stop", repoURL, remote)
	watcherLock.Lock()
	watcherMap[watcher.testID] = watcher
	watcherLock.Unlock()

	// Concurrent StopWatcher calls with no watch() goroutine draining done
	// must never block and must never panic on double close.
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := server.TaskRequest{
				Options: &server.UserOptions{UnWatch: watcher.testID},
			}
			if err := StopWatcher(req); err == nil {
				successes.Add(1)
			}
		}()
	}

	doneWait := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneWait)
	}()

	select {
	case <-doneWait:
	case <-time.After(2 * time.Second):
		t.Fatal("StopWatcher blocked without a receiver on watcher.done")
	}

	if got := successes.Load(); got != 1 {
		t.Fatalf("expected exactly 1 StopWatcher call to succeed, got %d", got)
	}

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			watcher.Clean()
		}()
	}
	wg.Wait()
}

func TestWatcherRecordNewBuildShortHashAndConcurrentStatus(t *testing.T) {
	resetWatcherGlobals(t)

	repoURL, _, _ := initLocalOriginRepo(t)
	remote, err := git.NewRemoteRepository(repoURL, "main")
	if err != nil {
		t.Fatalf("NewRemoteRepository failed: %v", err)
	}
	watcher := newTestGitWatcher(t, "watch-hist", repoURL, remote)
	defer watcher.Clean()

	watcherLock.Lock()
	watcherMap[watcher.testID] = watcher
	watcherLock.Unlock()

	// Short/empty commit hashes must not panic or leak historyLock.
	id1 := watcher.recordNewBuild("")
	id2 := watcher.recordNewBuild("abc")
	if id1 != "watch-hist-0001" || id2 != "watch-hist-0002" {
		t.Fatalf("unexpected build IDs: %q, %q", id1, id2)
	}

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				tid := watcher.recordNewBuild(fmt.Sprintf("%040x", idx*100+j))
				UpdateWatcherTest(tid, server.Pass)
			}
		}(i)
	}
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 15; j++ {
				infos := WatcherStatus()
				for _, info := range infos {
					if len(info.Tests) > 0 {
						info.Tests[0].Status = "caller-mutated"
					}
					if len(info.Packs) > 0 {
						info.Packs[0] = "caller-mutated"
					}
				}
			}
		}()
	}
	wg.Wait()

	snap := watcher.Info()
	if len(snap.Packs) != 1 || snap.Packs[0] != "pack-initial" {
		t.Fatalf("caller mutation leaked into watcher.packHistory: %v", snap.Packs)
	}
	for _, ti := range snap.Tests {
		if ti.Status == "caller-mutated" {
			t.Fatalf("caller mutation leaked into watcher.testHistory: %+v", ti)
		}
	}
}

func TestWatcherInitTestDeepCopiesTaskRequestAndWaitsBeforeCloseLog(t *testing.T) {
	resetWatcherGlobals(t)

	repoURL, workDir, commit1 := initLocalOriginRepo(t)
	remote, err := git.NewRemoteRepository(repoURL, "main")
	if err != nil {
		t.Fatalf("NewRemoteRepository failed: %v", err)
	}
	watcher := newTestGitWatcher(t, "watch-copy", repoURL, remote)
	watcherLock.Lock()
	watcherMap[watcher.testID] = watcher
	watcherLock.Unlock()

	type capturedCall struct {
		req server.TaskRequest
		id  string
	}
	var mu sync.Mutex
	var calls []capturedCall
	enteredFirst := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondDone := make(chan struct{})

	forwardKCSFunc = func(c server.TaskRequest, id string) error {
		mu.Lock()
		callNum := len(calls) + 1
		calls = append(calls, capturedCall{req: c, id: id})
		mu.Unlock()

		if callNum == 1 {
			close(enteredFirst)
			<-releaseFirst
		} else if callNum == 2 {
			defer close(secondDone)
		}
		return fmt.Errorf("simulated forward failure %d", callNum)
	}

	watcher.InitTest()
	select {
	case <-enteredFirst:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first forwardKCSFunc call")
	}

	commit2 := commitToOriginRepo(t, workDir, 2)
	if updated, err := watcher.repo.Update(); err != nil || !updated {
		t.Fatalf("expected repo.Update() to detect second commit (updated=%v, err=%v)", updated, err)
	}

	watcher.InitTest()
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second forwardKCSFunc call")
	}

	// Clean() must wait for the still-blocked first forward goroutine before
	// closing watcher.log.
	cleanDone := make(chan struct{})
	go func() {
		watcher.Clean()
		close(cleanDone)
	}()

	select {
	case <-cleanDone:
		t.Fatal("expected watcher.Clean() to wait for in-flight forwardKCSFunc goroutine")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseFirst)
	select {
	case <-cleanDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watcher.Clean() to finish")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("expected 2 forward calls, got %d", len(calls))
	}
	if calls[0].req.Options == calls[1].req.Options {
		t.Fatal("expected distinct UserOptions pointers per InitTest call")
	}
	if calls[0].req.ExtraOptions == calls[1].req.ExtraOptions {
		t.Fatal("expected distinct InternalOptions pointers per InitTest call")
	}
	if calls[0].req.Options.CommitID != commit1 || calls[1].req.Options.CommitID != commit2 {
		t.Fatalf("unexpected CommitIDs: got (%q, %q), want (%q, %q)",
			calls[0].req.Options.CommitID, calls[1].req.Options.CommitID, commit1, commit2)
	}
	if calls[0].req.ExtraOptions.TestID != "watch-copy-0001" || calls[1].req.ExtraOptions.TestID != "watch-copy-0002" {
		t.Fatalf("unexpected ExtraOptions.TestIDs: got (%q, %q)",
			calls[0].req.ExtraOptions.TestID, calls[1].req.ExtraOptions.TestID)
	}

	logBytes, err := os.ReadFile(watcher.logFile)
	if err != nil {
		t.Fatalf("failed to read watcher log file: %v", err)
	}
	logText := string(logBytes)
	if !strings.Contains(logText, "simulated forward failure 1") || !strings.Contains(logText, "simulated forward failure 2") {
		t.Fatalf("expected watcher log to capture both forward errors before CloseLog, got:\n%s", logText)
	}
}

func TestShardAndSharderConcurrentStatus(t *testing.T) {
	resetSharderGlobals(t)

	dir := t.TempDir()
	log := logging.InitLogger(filepath.Join(dir, "run.log"))
	defer logging.CloseLog(log)

	sharder := &ShardScheduler{
		testID:        "sharder-race",
		origCmd:       "gce-xfstests -c ext4/4k smoke",
		kernelVersion: "6.12.0-rc1",
		testResult:    server.DefaultResult,
		log:           log,
		logDir:        dir + "/",
	}
	shard := &ShardWorker{
		sharder:     sharder,
		shardID:     "aa",
		config:      "ext4/4k",
		zone:        "us-central1-f",
		vmStatus:    "waiting for launch",
		vmtestStart: time.Now(),
		testResult:  server.DefaultResult,
		log:         log.WithField("shardID", "aa"),
	}
	sharder.shards = []*ShardWorker{shard}

	sharderLock.Lock()
	sharderMap[sharder.testID] = sharder
	sharderLock.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(3)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				shard.setStatus(fmt.Sprintf("running-%d-%d", idx, j))
				shard.setStart(time.Now())
				shard.setStatusAndResult("completed", server.Pass)
			}
		}(i)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				sharder.setKernelVersion(fmt.Sprintf("6.12.%d", j))
				sharder.setResult(server.Pass)
			}
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				_ = SharderStatus()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer shard.exit()
		shard.setStatusAndResult("launching", server.DefaultResult)
		panic("simulated shard failure")
	}()

	wg.Wait()
}

func TestReserveSharderIDRejectsDuplicates(t *testing.T) {
	resetSharderGlobals(t)

	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if reserveSharderID("dup-test-id") {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := wins.Load(); got != 1 {
		t.Fatalf("expected exactly 1 reservation winner, got %d", got)
	}

	_, err := NewShardScheduler(server.TaskRequest{Options: &server.UserOptions{}}, "dup-test-id")
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("expected duplicate NewShardScheduler to fail with 'already running', got %v", err)
	}
}

func TestSharderPanicWritesReportBeforeCloseLog(t *testing.T) {
	resetSharderGlobals(t)
	resetWatcherGlobals(t)

	dir := t.TempDir()
	logFile := filepath.Join(dir, "run.log")
	log := logging.InitLogger(logFile)

	sharder := &ShardScheduler{
		testID:        "sharder-panic",
		origCmd:       "gce-xfstests -c ext4/4k smoke",
		kernelVersion: "6.12.0",
		testResult:    server.DefaultResult,
		log:           log,
		logDir:        dir + "/",
		logFile:       logFile,
		aggDir:        filepath.Join(dir, "agg") + "/",
		aggFile:       filepath.Join(dir, "results.ltm-sharder-panic"),
	}
	sharderLock.Lock()
	sharderMap[sharder.testID] = sharder
	sharderLock.Unlock()

	sharder.Run()

	logBytes, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read sharder run.log: %v", err)
	}
	logText := string(logBytes)
	if !strings.Contains(logText, "No results available for any of the shards") {
		t.Fatalf("expected run.log to contain panic message, got:\n%s", logText)
	}
	if !strings.Contains(logText, "Something failed, get stack trace") {
		t.Fatalf("expected email.ReportFailure stack trace to be flushed before CloseLog, got:\n%s", logText)
	}
	if got := SharderStatus(); len(got) != 0 {
		t.Fatalf("expected sharder.clean() to remove failed sharder from sharderMap, got %d", len(got))
	}
}
