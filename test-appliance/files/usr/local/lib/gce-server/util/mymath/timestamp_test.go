package mymath

import (
	"sync"
	"testing"
	"time"
)

func TestBlockedTimeStamps(t *testing.T) {
	var wg sync.WaitGroup
	start := time.Now()
	timestamps := make([]string, 5)
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func(i int) {
			timestamps[i] = GetTimeStamp()
			wg.Done()
		}(i)
	}
	wg.Wait()
	duration := time.Since(start)
	if duration < 4*time.Second || duration > 5*time.Second {
		t.Errorf("blocked timestamp requests should finish within 4~5s but takes %fs",
			duration.Seconds())
	}
	for i := 0; i < 4; i++ {
		if timestamps[i] == timestamps[i+1] {
			t.Error("Get duplicated timestamps")
		}
	}

}

func TestUnblockedTimeStamps(t *testing.T) {
	time.Sleep(1 * time.Second)
	for i := 0; i < 3; i++ {
		start := time.Now()
		GetTimeStamp()
		duration := time.Since(start)
		if duration > 500*time.Millisecond {
			t.Errorf("unblocked timestamp requests takes too long: %fs",
				duration.Seconds())
		}
		time.Sleep(2 * time.Second)
	}
}

func TestGetTimeStampMonotonicAcrossClockRollbackAndUTC(t *testing.T) {
	tsMu.Lock()
	origNow := nowFunc
	origSleep := sleepFunc
	origLast := lastUnix
	tsMu.Unlock()

	t.Cleanup(func() {
		tsMu.Lock()
		nowFunc = origNow
		sleepFunc = origSleep
		lastUnix = origLast
		tsMu.Unlock()
	})

	// Pick a timestamp inside a US Eastern autumn DST fallback window and
	// roll the clock backwards mid-stream to verify UTC + monotonicity.
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		ny = time.FixedZone("EDT", -4*3600)
	}
	var mu sync.Mutex
	cur := time.Date(2026, 11, 1, 1, 59, 59, 0, ny)

	tsMu.Lock()
	lastUnix = 0
	nowFunc = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return cur
	}
	sleepFunc = func(d time.Duration) {}
	tsMu.Unlock()

	t1 := GetTimeStamp()
	// Simulate wall clock stepping backwards by an hour.
	mu.Lock()
	cur = cur.Add(-1 * time.Hour)
	mu.Unlock()
	t2 := GetTimeStamp()
	t3 := GetTimeStamp()

	if !(t1 < t2 && t2 < t3) {
		t.Fatalf("expected strictly increasing UTC timestamps across rollback, got %q, %q, %q", t1, t2, t3)
	}
}
