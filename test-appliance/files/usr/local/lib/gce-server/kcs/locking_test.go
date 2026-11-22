package main

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"thunk.org/gce-server/util/git"
	"thunk.org/gce-server/util/logging"
	"thunk.org/gce-server/util/server"
)

func newTestBisector(t *testing.T, id string, lastActive time.Time) *GitBisector {
	t.Helper()
	dir := t.TempDir()
	resultsDir := filepath.Join(dir, "results") + "/"
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		t.Fatal(err)
	}
	log := logging.InitLogger(filepath.Join(dir, "run.log"))
	return &GitBisector{
		testID:      id,
		origCmd:     "gce-xfstests -c ext4/4k smoke",
		testRequest: server.TaskRequest{Options: &server.UserOptions{GitRepo: "https://example.com/linux.git"}},
		badCommit:   "badc0ffee",
		goodCommits: []string{"11111111", "22222222"},
		lastActive:  lastActive,
		done:        make(chan struct{}),
		logDir:      dir + "/",
		resultsDir:  resultsDir,
		log:         log,
	}
}

func resetBisectorGlobals(t *testing.T) {
	t.Helper()
	bisectorLock.Lock()
	origMap := bisectorMap
	origPending := pendingBisectors
	bisectorMap = make(map[string]*GitBisector)
	pendingBisectors = make(map[string]bool)
	bisectorLock.Unlock()

	t.Cleanup(func() {
		bisectorLock.Lock()
		bisectorMap = origMap
		pendingBisectors = origPending
		bisectorLock.Unlock()
	})
}

func TestBisectorExpiryAndConcurrentClean(t *testing.T) {
	resetBisectorGlobals(t)

	expired := newTestBisector(t, "bisect-expired", time.Now().Add(-bisectorTimeout-time.Minute))
	bisectorLock.Lock()
	bisectorMap[expired.testID] = expired
	bisectorLock.Unlock()

	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		<-expired.done
	}()

	cleanedCh := make(chan bool, 1)
	go func() {
		cleanedCh <- expired.CheckActive()
	}()

	select {
	case cleaned := <-cleanedCh:
		if !cleaned {
			t.Fatal("expected CheckActive() to clean expired bisector")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CheckActive() deadlocked on expired bisector cleanup")
	}

	select {
	case <-monitorDone:
	case <-time.After(2 * time.Second):
		t.Fatal("monitor goroutine did not observe closed done channel")
	}

	if got := BisectorStatus(); len(got) != 0 {
		t.Fatalf("expected 0 active bisectors after timeout cleanup, got %d", len(got))
	}
	if _, err := os.Stat(expired.resultsDir); !os.IsNotExist(err) {
		t.Fatalf("expected resultsDir to be removed on Clean(), stat err=%v", err)
	}

	// Verify concurrent Clean(), CheckActive(), and BisectorStatus() + slice
	// mutation neither panic nor race.
	active := newTestBisector(t, "bisect-concurrent", time.Now())
	bisectorLock.Lock()
	bisectorMap[active.testID] = active
	bisectorLock.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				infos := BisectorStatus()
				for _, info := range infos {
					if len(info.GoodCommits) > 0 {
						info.GoodCommits[0] = "mutated-by-caller"
					}
				}
			}
		}()
		go func() {
			defer wg.Done()
			_ = active.CheckActive()
		}()
		go func() {
			defer wg.Done()
			active.Clean()
		}()
	}
	wg.Wait()

	info := active.Info()
	if len(info.GoodCommits) == 0 || info.GoodCommits[0] != "11111111" {
		t.Fatalf("caller slice mutation leaked into GitBisector.goodCommits: %v", info.GoodCommits)
	}
}

func TestBisectorActiveOpPreventsExpiryCleanup(t *testing.T) {
	resetBisectorGlobals(t)

	b := newTestBisector(t, "bisect-in-flight", time.Now())
	defer b.Clean()

	bisectorLock.Lock()
	bisectorMap[b.testID] = b
	bisectorLock.Unlock()

	if !b.beginOp() {
		t.Fatal("expected beginOp() to succeed on live bisector")
	}
	b.stateMu.Lock()
	b.lastActive = time.Now().Add(-bisectorTimeout - time.Minute)
	b.stateMu.Unlock()

	if b.CheckActive() {
		t.Fatal("expected CheckActive() not to clean bisector while an operation is active")
	}

	b.endOp()
	b.stateMu.Lock()
	b.lastActive = time.Now().Add(-bisectorTimeout - time.Minute)
	b.stateMu.Unlock()

	if !b.CheckActive() {
		t.Fatal("expected CheckActive() to clean expired bisector once active operation ended")
	}
	if b.beginOp() {
		t.Fatal("expected beginOp() to fail after Clean()")
	}
}

func TestReserveBisectorIDAndHasActiveOrPending(t *testing.T) {
	resetBisectorGlobals(t)

	if HasActiveOrPendingBisectors() {
		t.Fatal("expected no active or pending bisectors initially")
	}

	var wins int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if reserveBisectorID("dup-bisect") {
				atomic.AddInt32(&wins, 1)
			}
		}()
	}
	wg.Wait()

	if wins != 1 {
		t.Fatalf("expected exactly 1 reservation winner, got %d", wins)
	}
	if !HasActiveOrPendingBisectors() {
		t.Fatal("expected pending bisector reservation to report active/pending")
	}

	bisectorLock.Lock()
	delete(pendingBisectors, "dup-bisect")
	bisectorLock.Unlock()

	if HasActiveOrPendingBisectors() {
		t.Fatal("expected HasActiveOrPendingBisectors() to be false after clearing pending map")
	}
}

func TestStartTrackerTaskTrackingAndStop(t *testing.T) {
	resetBisectorGlobals(t)

	trackerMu.Lock()
	origTimeout := buildTimeout
	origShutdown := shutdownFunc
	origShuttingDown := shuttingDown
	origActive := activeTasks
	origLast := lastActivity
	trackerMu.Unlock()

	t.Cleanup(func() {
		trackerMu.Lock()
		buildTimeout = origTimeout
		shutdownFunc = origShutdown
		shuttingDown = origShuttingDown
		activeTasks = origActive
		lastActivity = origLast
		trackerMu.Unlock()
		select {
		case <-newBuild:
		default:
		}
	})

	// Repeated notifyNewBuild calls without a consumer must never block.
	notifyDone := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			notifyNewBuild()
		}
		close(notifyDone)
	}()
	select {
	case <-notifyDone:
	case <-time.After(time.Second):
		t.Fatal("notifyNewBuild blocked without a consumer")
	}

	inst, err := server.New("127.0.0.1:0", "kcs-test")
	if err != nil {
		t.Fatalf("server.New failed: %v", err)
	}
	defer inst.Close()

	// Part 1: active task blocks auto-shutdown across buildTimeout; EndTask
	// allows idle shutdown and rejects subsequent TryBeginTask calls.
	shutdownCh := make(chan struct{}, 1)
	trackerMu.Lock()
	buildTimeout = 40 * time.Millisecond
	shuttingDown = false
	activeTasks = 0
	lastActivity = time.Now()
	shutdownFunc = func(*server.Instance) {
		shutdownCh <- struct{}{}
	}
	trackerMu.Unlock()

	if !TryBeginTask() {
		t.Fatal("TryBeginTask() failed before tracker start")
	}

	stop1 := make(chan struct{})
	finished1 := make(chan struct{})
	go StartTracker(inst, stop1, finished1)

	select {
	case <-shutdownCh:
		EndTask()
		close(stop1)
		t.Fatal("StartTracker shut down while an active task was still running")
	case <-time.After(140 * time.Millisecond):
	}

	EndTask()
	WaitTasks()

	select {
	case <-shutdownCh:
	case <-time.After(2 * time.Second):
		close(stop1)
		t.Fatal("timed out waiting for idle shutdown after EndTask()")
	}
	select {
	case <-finished1:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for StartTracker to exit after shutdown")
	}
	if TryBeginTask() {
		EndTask()
		t.Fatal("expected TryBeginTask() to return false once shuttingDown is set")
	}

	// Part 2: closing stop channel exits StartTracker immediately without
	// invoking shutdownFunc.
	var unexpectedShutdown atomic.Int32
	trackerMu.Lock()
	buildTimeout = 5 * time.Second
	shuttingDown = false
	activeTasks = 0
	lastActivity = time.Now()
	shutdownFunc = func(*server.Instance) {
		unexpectedShutdown.Add(1)
	}
	trackerMu.Unlock()

	stop2 := make(chan struct{})
	finished2 := make(chan struct{})
	go StartTracker(inst, stop2, finished2)
	close(stop2)

	select {
	case <-finished2:
	case <-time.After(2 * time.Second):
		t.Fatal("StartTracker did not exit promptly when stop channel was closed")
	}
	if unexpectedShutdown.Load() != 0 {
		t.Fatal("shutdownFunc should not be called when stop channel is closed")
	}
}

func TestGetRepoBuildLockPerRepoIsolation(t *testing.T) {
	repoLock.Lock()
	origMap := repoMap
	origLocks := repoBuildLocks
	repoMap = make(map[string]*git.Repository)
	repoBuildLocks = make(map[string]*sync.Mutex)
	repoLock.Unlock()

	t.Cleanup(func() {
		repoLock.Lock()
		repoMap = origMap
		repoBuildLocks = origLocks
		repoLock.Unlock()
	})

	const n = 16
	locksA := make([]*sync.Mutex, n)
	locksB := make([]*sync.Mutex, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			locksA[idx] = getRepoBuildLock("repo-a")
		}(i)
		go func(idx int) {
			defer wg.Done()
			locksB[idx] = getRepoBuildLock("repo-b")
		}(i)
	}
	wg.Wait()

	for i := 1; i < n; i++ {
		if locksA[i] != locksA[0] {
			t.Fatal("expected identical mutex pointer for repo-a")
		}
		if locksB[i] != locksB[0] {
			t.Fatal("expected identical mutex pointer for repo-b")
		}
	}
	if locksA[0] == locksB[0] {
		t.Fatal("expected distinct mutexes for distinct repositories")
	}
}

func TestBisectFailReceiverPrefersReportFailEmail(t *testing.T) {
	if got := bisectFailReceiver(nil); got != "" {
		t.Fatalf("expected empty receiver for nil options, got %q", got)
	}
	if got := bisectFailReceiver(&server.UserOptions{
		ReportEmail:     "",
		ReportFailEmail: "fail-only@example.com",
	}); got != "fail-only@example.com" {
		t.Fatalf("expected ReportFailEmail when ReportEmail is empty, got %q", got)
	}
	if got := bisectFailReceiver(&server.UserOptions{
		ReportEmail:     "normal@example.com",
		ReportFailEmail: "alerts@example.com",
	}); got != "alerts@example.com" {
		t.Fatalf("expected ReportFailEmail to take precedence, got %q", got)
	}
	if got := bisectFailReceiver(&server.UserOptions{
		ReportEmail: "normal@example.com",
	}); got != "normal@example.com" {
		t.Fatalf("expected fallback to ReportEmail, got %q", got)
	}
}
