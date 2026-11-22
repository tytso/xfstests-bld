package email

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
