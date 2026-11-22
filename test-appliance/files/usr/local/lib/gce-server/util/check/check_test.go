package check

import (
	"io"
	"os/exec"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestValidateTestID(t *testing.T) {
	validIDs := []string{
		"20261122025400",
		"20261122025400-0001",
		"20261122025400-c870e04e",
		"test",
		"test-20261122-amd64",
		"testkcs-20261122-arm64",
		"test.20261122",
		"my_test_run_01",
		"v6.18.22-rc1",
		"github.com-XiaoyangShen-spinner_test.git-381e3d6d",
		"git.kernel.org-elder-linux.git-7353b8c3",
	}

	for _, id := range validIDs {
		if err := ValidateTestID(id); err != nil {
			t.Errorf("expected ValidateTestID(%q) to succeed, got error: %v", id, err)
		}
	}

	invalidIDs := []string{
		"",
		"-",
		"-test",
		"--foo",
		"/",
		"\\",
		".",
		"..",
		"../etc",
		"foo/..",
		"foo/bar",
		"/etc/passwd",
		"foo\\bar",
		"foo..bar",
		".../foo",
		"foo\x00bar",
		"foo bar",
		"foo\tbar",
		"foo\nbar",
		"foo\rbar",
		" test",
		"test ",
		"\x1b[31mtest",
		"test\x7f",
	}

	for _, id := range invalidIDs {
		if err := ValidateTestID(id); err == nil {
			t.Errorf("expected ValidateTestID(%q) to fail, but got nil error", id)
		}
	}
}

func TestValidPassword(t *testing.T) {
	if !ValidPassword("s3cr3tP4ssw0rd!", "s3cr3tP4ssw0rd!") {
		t.Error("expected matching non-empty password to succeed")
	}
	if ValidPassword("wrong", "s3cr3tP4ssw0rd!") {
		t.Error("expected mismatched password to fail")
	}
	if ValidPassword("", "s3cr3tP4ssw0rd!") {
		t.Error("expected empty provided password to fail")
	}
	if ValidPassword("", "") {
		t.Error("expected empty expected password to fail")
	}
	if ValidPassword("s3cr3t", "") {
		t.Error("expected non-empty provided with empty expected password to fail")
	}
}

func TestLimitedRunAndOutputReleaseCapperOnLimiterError(t *testing.T) {
	origLimiter := limiter
	t.Cleanup(func() {
		limiter = origLimiter
	})

	// A burst of 0 causes limiter.Wait() to return an error immediately.
	limiter = rate.NewLimiter(rate.Every(time.Second), 0)

	for i := 0; i < cmdCap*2; i++ {
		if err := LimitedRun(exec.Command("true"), "", EmptyEnv, io.Discard, io.Discard); err == nil {
			t.Fatal("expected LimitedRun to fail when limiter burst is 0")
		}
		if _, err := LimitedOutput(exec.Command("true"), "", EmptyEnv, io.Discard); err == nil {
			t.Fatal("expected LimitedOutput to fail when limiter burst is 0")
		}
	}

	if got := len(capper); got != 0 {
		t.Fatalf("expected 0 leaked capper slots, got %d", got)
	}
}

func TestLimitedRunConcurrentDoesNotLeakSlots(t *testing.T) {
	origLimiter := limiter
	t.Cleanup(func() {
		limiter = origLimiter
	})
	limiter = rate.NewLimiter(rate.Inf, cmdCap)

	var wg sync.WaitGroup
	for i := 0; i < cmdCap*2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := LimitedRun(exec.Command("true"), "", EmptyEnv, io.Discard, io.Discard); err != nil {
				t.Errorf("LimitedRun failed: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := len(capper); got != 0 {
		t.Fatalf("expected 0 leaked capper slots after concurrent runs, got %d", got)
	}
}

