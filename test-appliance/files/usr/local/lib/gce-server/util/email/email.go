/*
Package email sends test reports or failure logs with package sendgrid.
*/
package email

import (
	"fmt"
	"io/ioutil"
	"runtime/debug"
	"strings"

	"thunk.org/gce-server/util/check"
	"thunk.org/gce-server/util/gcp"
	"thunk.org/gce-server/util/logging"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/sirupsen/logrus"
)

// Send sends an email with subject and content to the receivers.
// Sender is configured to be receiver if not set in config.
func Send(subject string, content string, receivers string) error {
	if receivers == "" {
		return fmt.Errorf("No destination for report to be sent to")
	}
	receiversSlice := strings.Split(receivers, ",")

	apiKey, err := gcp.GceConfig.Get("SENDGRID_API_KEY")
	if err != nil {
		return err
	}
	sender, err := gcp.GceConfig.Get("GCE_REPORT_SENDER")
	if err != nil {
		sender = receiversSlice[0]
	}

	m := mail.NewV3Mail()
	from := mail.NewEmail("Xfstests Reporter", sender)
	m.SetFrom(from)
	m.Subject = subject

	p := mail.NewPersonalization()

	for _, receiver := range receiversSlice {
		to := mail.NewEmail("", receiver)
		p.AddTos(to)
	}

	m.AddPersonalizations(p)

	c := mail.NewContent("text/plain", content)
	m.AddContent(c)

	client := sendgrid.NewSendClient(apiKey)
	response, err := client.Send(m)

	if err != nil {
		return err
	}

	if response.StatusCode >= 200 && response.StatusCode <= 299 {
		return nil
	}
	return fmt.Errorf("Send failed with code %d, response: %s", response.StatusCode, response.Body)
}

// ReportFailure catches panic and sends a failure report email to user.
// If log writes to the same location as logFile, flush the log to disk first.
// Only works as a deferred function.
func ReportFailure(log *logrus.Entry, logFile string, email string, subject string) {
	if r := recover(); r != nil {
		log.Error("Something failed, get stack trace")
		log.Error(string(debug.Stack()))
		if email == "" {
			log.Info("No email receiver provided")
			return
		}
		log.Info("Sending failure report")

		msg := "unknown panic"
		switch s := r.(type) {
		case string:
			msg = s
		case error:
			msg = s.Error()
		case *logrus.Entry:
			msg = s.Message
		}

		file := logging.GetFile(log)
		if file.Name() != "" && file.Name() == logFile {
			file.Sync()
		}

		if check.FileExists(logFile) {
			log.Debug("Reading log file to be sent")
			content, err := ioutil.ReadFile(logFile)
			if check.NoError(err, log, "Failed to read log file") {
				msg = msg + "\n\n" + string(content)
			}
		}
		err := Send(subject, msg, email)
		check.NoError(err, log, "Failed to send the email")
	}
}
