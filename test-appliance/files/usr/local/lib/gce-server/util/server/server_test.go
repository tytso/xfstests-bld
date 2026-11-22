package server

import (
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
