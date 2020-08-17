package email

import (
	"gce-server/util/gcp"
	"os"
	"testing"
)

func TestEmail(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Error(err)
	}
	if hostname != "xfstests-ltm" && hostname != "xfstests-kcs" {
		t.Skip("test only runs on LTM or KCS server")
	}

	receiver, err := gcp.GceConfig.Get("GCE_REPORT_EMAIL")
	if err != nil {
		t.Error(err)
	}
	msg := "test msg"
	err = Send("test", msg, receiver)
	if err != nil {
		t.Error(err)
	}
}
