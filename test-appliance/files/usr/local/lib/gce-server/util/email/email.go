/*
Package email sends test reports or failure logs.
*/
package email

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"runtime/debug"
	"strings"

	"thunk.org/gce-server/util/check"
	"thunk.org/gce-server/util/logging"

	"github.com/sirupsen/logrus"
)

var sendMailPath = "/usr/local/sbin/send_mail.py"

// Send sends an email with subject and content to the receivers.
func Send(subject string, content string, receivers string) error {
	if receivers == "" {
		return fmt.Errorf("No destination for report to be sent to")
	}
	if strings.ContainsAny(subject, "\r\n\x00") {
		return fmt.Errorf("email subject must not contain CR, LF, or NUL")
	}
	if err := check.ValidateEmailList(receivers); err != nil {
		return fmt.Errorf("invalid email receivers: %w", err)
	}

	cmd := exec.Command(sendMailPath,
		"-s", subject,
		"--",
		receivers,
	)
	cmd.Stdin = strings.NewReader(content)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("send_mail.py failed: %v, output: %s", err, string(output))
	}
	return nil
}

func reportRecoveredFailure(r interface{}, log *logrus.Entry, logFile string, email string, subject string) {
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
	if file != nil && file.Name() != "" && file.Name() == logFile {
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

// ReportFailure catches panic and sends a failure report email to user.
// If log writes to the same location as logFile, flush the log to disk first.
// Only works as a deferred function.
func ReportFailure(log *logrus.Entry, logFile string, email string, subject string) {
	if r := recover(); r != nil {
		reportRecoveredFailure(r, log, logFile, email, subject)
	}
}

// ReportFailureFunc works like ReportFailure but evaluates subjectFn lazily
// when a panic is recovered, so subject strings can reflect state updated
// during execution. Only works as a deferred function.
func ReportFailureFunc(log *logrus.Entry, logFile string, email string, subjectFn func() string) {
	if r := recover(); r != nil {
		subject := ""
		if subjectFn != nil {
			subject = subjectFn()
		}
		reportRecoveredFailure(r, log, logFile, email, subject)
	}
}
