package email

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestSendValidatesInputsAndPassesOptionTerminator(t *testing.T) {
	tmpDir := t.TempDir()
	argsFile := filepath.Join(tmpDir, "args.txt")
	bodyFile := filepath.Join(tmpDir, "body.txt")
	stubScript := filepath.Join(tmpDir, "send_mail_stub.sh")

	scriptContent := "#!/bin/bash\nset -euo pipefail\nprintf '%s\\n' \"$@\" > " + argsFile + "\ncat > " + bodyFile + "\n"
	if err := os.WriteFile(stubScript, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	origPath := sendMailPath
	sendMailPath = stubScript
	t.Cleanup(func() {
		sendMailPath = origPath
	})

	for _, bad := range []string{"", "-f /etc/shadow", "--config=/etc/passwd", "bad\n@example.com"} {
		if err := Send("xfstests report", "body", bad); err == nil {
			t.Fatalf("expected Send with receivers=%q to fail", bad)
		}
	}
	if err := Send("bad\r\nsubject", "body", "tytso@mit.edu"); err == nil {
		t.Fatal("expected Send with CRLF in subject to fail")
	}

	if err := Send("xfstests results", "hello world\n", "tytso@mit.edu, user@google.com"); err != nil {
		t.Fatalf("Send failed unexpectedly: %v", err)
	}

	gotArgs, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := "-s\nxfstests results\n--\ntytso@mit.edu, user@google.com\n"
	if string(gotArgs) != wantArgs {
		t.Fatalf("unexpected send_mail.py args:\ngot:  %q\nwant: %q", string(gotArgs), wantArgs)
	}
	gotBody, err := os.ReadFile(bodyFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gotBody), "hello world") {
		t.Fatalf("unexpected stdin body: %q", string(gotBody))
	}
}

func TestReportFailureFuncEvaluatesSubjectLazilyOnPanic(t *testing.T) {
	tmpDir := t.TempDir()
	argsFile := filepath.Join(tmpDir, "args.txt")
	bodyFile := filepath.Join(tmpDir, "body.txt")
	stubScript := filepath.Join(tmpDir, "send_mail_stub.sh")

	scriptContent := "#!/bin/bash\nset -euo pipefail\nprintf '%s\\n' \"$@\" > " + argsFile + "\ncat > " + bodyFile + "\n"
	if err := os.WriteFile(stubScript, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	origPath := sendMailPath
	sendMailPath = stubScript
	t.Cleanup(func() {
		sendMailPath = origPath
	})

	logger := logrus.New()
	var buf bytes.Buffer
	logger.Out = &buf
	entry := logrus.NewEntry(logger)

	called := 0
	kver := "unknown_kernel_version"

	// Non-panicking execution must not evaluate subjectFn.
	func() {
		defer ReportFailureFunc(entry, "", "tytso@mit.edu", func() string {
			called++
			return "xfstests failure ltm-1 " + kver
		})
		kver = "6.14.0-rc1"
	}()
	if called != 0 {
		t.Fatalf("expected subjectFn not to be called without panic, got %d", called)
	}

	// Panicking execution must recover and use the updated kver in subject.
	kver = "unknown_kernel_version"
	func() {
		defer ReportFailureFunc(entry, "", "tytso@mit.edu", func() string {
			called++
			return "xfstests failure ltm-1 " + kver
		})
		kver = "6.14.0-rc2"
		panic("simulated failure after shard boot")
	}()
	if called != 1 {
		t.Fatalf("expected subjectFn to be called once on panic, got %d", called)
	}
	gotArgs, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := "-s\nxfstests failure ltm-1 6.14.0-rc2\n--\ntytso@mit.edu\n"
	if string(gotArgs) != wantArgs {
		t.Fatalf("unexpected send_mail.py args:\ngot:  %q\nwant: %q", string(gotArgs), wantArgs)
	}
}
