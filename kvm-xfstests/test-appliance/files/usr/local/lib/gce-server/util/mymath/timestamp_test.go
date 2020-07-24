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
