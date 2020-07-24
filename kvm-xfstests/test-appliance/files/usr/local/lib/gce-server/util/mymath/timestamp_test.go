package mymath

import (
	"testing"
	"time"
)

func TestTimeStamp(t *testing.T) {
	for i := 0; i < 5; i++ {
		go func() {
			t.Log(GetTimeStamp())
		}()
	}
	time.Sleep(5 * time.Second)
	for i := 0; i < 5; i++ {
		t.Log(GetTimeStamp())
		time.Sleep(5 * time.Second)
	}
}
