package check

import (
	"testing"
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
