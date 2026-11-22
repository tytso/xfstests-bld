package email_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"thunk.org/gce-server/util/email"
	"thunk.org/gce-server/util/gcp"
)

func TestEmail(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Error(err)
	}
	if hostname != "xfstests-ltm" && hostname != "xfstests-kcs" {
		t.Skip("test only runs on LTM or KCS server")
	}

	receiver, err := gcp.GceConfig().Get("GCE_REPORT_EMAIL")
	if err != nil {
		t.Error(err)
	}
	msg := "test msg"
	err = email.Send("test", msg, receiver)
	if err != nil {
		t.Error(err)
	}
}

func TestReportFailureNonFileLogger(t *testing.T) {
	logger := logrus.New()
	var buf bytes.Buffer
	logger.Out = &buf
	entry := logrus.NewEntry(logger)

	func() {
		defer email.ReportFailure(entry, "/nonexistent/run.log", "", "test subject")
		panic("boom")
	}()

	if buf.Len() == 0 {
		t.Fatal("expected ReportFailure to log panic details without nil-dereferencing GetFile")
	}
}
