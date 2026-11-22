/*
Package mymath implements math functions and generates unique timestamps.
*/
package mymath

import (
	"fmt"
	"sync"
	"time"
)

var (
	tsMu      sync.Mutex
	lastUnix  int64
	nowFunc   = time.Now
	sleepFunc = time.Sleep
)

// GetTimeStamp returns a unique, strictly increasing UTC timestamp.
func GetTimeStamp() string {
	tsMu.Lock()
	defer tsMu.Unlock()

	sec := nowFunc().UTC().Unix()
	if sec == lastUnix {
		sleepFunc(time.Second)
		sec = nowFunc().UTC().Unix()
	}
	if sec <= lastUnix {
		sec = lastUnix + 1
	}
	lastUnix = sec

	t := time.Unix(sec, 0).UTC()
	return fmt.Sprintf("%.4d%.2d%.2d%.2d%.2d%.2d",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
}

// MinInt returns the smaller int.
func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// MaxInt returns the larger int.
func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// MaxIntSlice returns the largest int in a slice.
func MaxIntSlice(slice []int) (int, error) {
	if len(slice) == 0 {
		return 0, fmt.Errorf("MaxIntSlice: empty slice")
	}
	max := slice[0]
	for _, i := range slice[1:] {
		max = MaxInt(max, i)
	}
	return max, nil
}

// MinIntSlice returns the smallest int in a slice.
func MinIntSlice(slice []int) (int, error) {
	if len(slice) == 0 {
		return 0, fmt.Errorf("MaxIntSlice: empty slice")
	}
	max := slice[0]
	for _, i := range slice[1:] {
		max = MinInt(max, i)
	}
	return max, nil
}
