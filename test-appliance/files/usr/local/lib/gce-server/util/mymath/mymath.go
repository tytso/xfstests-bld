/*
Package mymath implements math functions and generates unique timestamps.
*/
package mymath

import (
	"fmt"
	"time"
)

var (
	query     chan struct{}
	timestamp chan time.Time
)

func init() {
	query = make(chan struct{})
	timestamp = make(chan time.Time)

	go func() {
		for {
			<-query
			timestamp <- time.Now()
			time.Sleep(1100 * time.Millisecond)
		}
	}()
}

// GetTimeStamp returns a unique current timestamp.
func GetTimeStamp() string {
	query <- struct{}{}
	t := <-timestamp
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
