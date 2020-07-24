/*
Package mymath implements math functions and generates unique timestamps.
*/
package mymath

import (
	"fmt"
	"sync"
	"time"
)

var idMutex sync.Mutex

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

// GetTimeStamp returns the current timestamp
// Guaranteed uniqueness across go routines.
func GetTimeStamp() string {
	idMutex.Lock()
	defer idMutex.Unlock()
	// TODO: avoid duplicate timestamp with more efficient ways
	time.Sleep(2 * time.Second)
	t := time.Now()
	return fmt.Sprintf("%.4d%.2d%.2d%.2d%.2d%.2d",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
}
