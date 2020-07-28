package email

import (
	"gce-server/util/gcp"
	"testing"
)

func TestEmail(t *testing.T) {
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
