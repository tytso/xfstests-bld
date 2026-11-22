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

func TestValidateGitRepoURL(t *testing.T) {
	validURLs := []string{
		"https://github.com/tytso/ext4.git",
		"https://github.com/XiaoyangShen/spinner_test.git",
		"git://git.kernel.org/pub/scm/linux/kernel/git/elder/linux.git",
		"https://kernel.googlesource.com/pub/scm/linux/kernel/git/stable/linux.git",
		"https://kernel.googlesource.com/linux.git",
		"http://git.example.com:8080/linux.git/",
	}
	for _, u := range validURLs {
		if err := ValidateGitRepoURL(u); err != nil {
			t.Errorf("expected ValidateGitRepoURL(%q) to succeed, got: %v", u, err)
		}
	}

	invalidURLs := []string{
		"",
		"-u",
		"--upload-pack=/bin/sh",
		"file:///etc/passwd",
		"file://localhost/tmp/repo.git",
		"ssh://git@github.com/tytso/ext4.git",
		"git@github.com:tytso/ext4.git",
		"ext::sh -c %G/evil",
		"https://github.com",
		"https://github.com/",
		"https:///tytso/ext4.git",
		"https://-evil.example.com/linux.git",
		"https://evil..example.com/linux.git",
		"https://user:pass@github.com/tytso/ext4.git",
		"https://github.com/tytso/ext4.git?foo=bar",
		"https://github.com/tytso/ext4.git#main",
		"https://github.com/tytso/../ext4.git",
		"https://github.com/tytso/./ext4.git",
		"https://github.com/tytso//ext4.git",
		"https://github.com/tytso/ext4..bad.git",
		"https://github.com/tytso/ext4\\bad.git",
		"https://github.com/tytso/ext4.git\x00",
		"https://github.com/tytso/ext4.git --upload-pack=evil",
		"https://github.com/tytso/ext4.git\n--upload-pack=evil",
	}
	for _, u := range invalidURLs {
		if err := ValidateGitRepoURL(u); err == nil {
			t.Errorf("expected ValidateGitRepoURL(%q) to fail, got nil", u)
		}
	}
}

func TestValidateGitRefAndBranch(t *testing.T) {
	validRefs := []string{
		"HEAD",
		"HEAD~2",
		"v5.6",
		"v6.12-rc1^1",
		"7111951b8d4973bda27ff663f2cf18b663d15b48",
		"origin/master",
		"ext4/for-next",
		"refs/tags/v6.12.1",
		"bisect-test-ext4-035",
	}
	for _, r := range validRefs {
		if err := ValidateGitRef(r); err != nil {
			t.Errorf("expected ValidateGitRef(%q) to succeed, got: %v", r, err)
		}
	}

	invalidRefs := []string{
		"",
		"-b",
		"--upload-pack=/bin/sh",
		"--format=%H",
		"--detach",
		"HEAD..origin/main",
		"HEAD...origin/main",
		"HEAD@{1}",
		"HEAD:Makefile",
		"/main",
		"origin/",
		"ext4//dev",
		"ext4/.hidden",
		"ext4/dev.lock",
		"v5.6.",
		"main*",
		"main?1",
		"main[0]",
		"foo\\bar",
		"foo bar",
		"foo\nbar",
		"foo\x00bar",
	}
	for _, r := range invalidRefs {
		if err := ValidateGitRef(r); err == nil {
			t.Errorf("expected ValidateGitRef(%q) to fail, got nil", r)
		}
	}

	validBranches := []string{
		"main",
		"master",
		"dev",
		"ext4/for-next",
		"refs/heads/main",
		"bisect-test-generic-307",
	}
	for _, b := range validBranches {
		if err := ValidateGitBranch(b); err != nil {
			t.Errorf("expected ValidateGitBranch(%q) to succeed, got: %v", b, err)
		}
	}

	invalidBranches := []string{
		"",
		"@",
		"HEAD~1",
		"main^",
		"--upload-pack=evil",
		"ext4/*",
		"origin..main",
	}
	for _, b := range invalidBranches {
		if err := ValidateGitBranch(b); err == nil {
			t.Errorf("expected ValidateGitBranch(%q) to fail, got nil", b)
		}
	}
}

func TestValidateBucketSubdirAndGSURI(t *testing.T) {
	validSubdirs := []string{
		"results",
		"results/ext4",
		"ltm-results/2026_11/arm64",
	}
	for _, s := range validSubdirs {
		if err := ValidateBucketSubdir(s); err != nil {
			t.Errorf("expected ValidateBucketSubdir(%q) to succeed, got: %v", s, err)
		}
	}

	invalidSubdirs := []string{
		"",
		"/",
		"/results",
		"results/",
		"results//ext4",
		"-results",
		"results/-ext4",
		".",
		"..",
		"results/../etc",
		"results/./ext4",
		"results/foo..bar",
		"results/*",
		"results/?",
		"results/[a]",
		"results\\ext4",
		"results ext4",
		"results\next4",
		"results\x00ext4",
	}
	for _, s := range invalidSubdirs {
		if err := ValidateBucketSubdir(s); err == nil {
			t.Errorf("expected ValidateBucketSubdir(%q) to fail, got nil", s)
		}
	}

	validURIs := []string{
		"gs://my-bucket/bzImage",
		"gs://my-bucket/kernel.deb",
		"gs://gce_xfstests.bucket-1/kernels/bzImage-test-20261122-onerun.deb",
		"gs://my-bucket/build_config",
	}
	for _, u := range validURIs {
		if err := ValidateGSURI(u); err != nil {
			t.Errorf("expected ValidateGSURI(%q) to succeed, got: %v", u, err)
		}
	}

	invalidURIs := []string{
		"",
		"/etc/passwd",
		"file:///etc/passwd",
		"https://storage.googleapis.com/my-bucket/bzImage",
		"-f",
		"gs://",
		"gs://my-bucket",
		"gs://my-bucket/",
		"gs://ab/bzImage",
		"gs://-bad/bzImage",
		"gs://BadUpper/bzImage",
		"gs://my..bucket/bzImage",
		"gs://my-bucket//bzImage",
		"gs://my-bucket/kernels/",
		"gs://my-bucket/../etc/passwd",
		"gs://my-bucket/kernels/./bzImage",
		"gs://my-bucket/kernels/foo..bar",
		"gs://my-bucket/kernels/*",
		"gs://my-bucket/kernels/bzImage\\bad",
		"gs://my-bucket/kernels/bzImage with space",
		"gs://my-bucket/kernels/bzImage\n",
		"gs://my-bucket/kernels/bzImage\x00",
	}
	for _, u := range invalidURIs {
		if err := ValidateGSURI(u); err == nil {
			t.Errorf("expected ValidateGSURI(%q) to fail, got nil", u)
		}
	}
}

