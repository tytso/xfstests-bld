package main

import (
	"fmt"

	"thunk.org/gce-server/util/check"
	"thunk.org/gce-server/util/email"
	"thunk.org/gce-server/util/logging"
	"thunk.org/gce-server/util/server"
)

// ForwardKCS forwards a build or git bisect start task to KCS.
// Sends failure report email to user on failures
func ForwardKCS(req server.TaskRequest, testID string) error {
	if err := check.ValidateTestID(testID); err != nil {
		return fmt.Errorf("invalid testID: %w", err)
	}
	if req.ExtraOptions != nil {
		if err := check.ValidateTestID(req.ExtraOptions.TestID); err != nil {
			return fmt.Errorf("invalid extra_options.test_id: %w", err)
		}
	}

	logDir := logging.LTMLogDir + testID + "/"
	err := check.CreateDir(logDir)
	if err != nil {
		return err
	}
	logFile := logDir + "run.log"
	log := logging.InitLogger(logFile)
	defer logging.CloseLog(log)

	subject := "xfstests LTM forwarding request failure " + testID
	defer email.ReportFailure(log, logFile, req.Options.ReportFailEmail, subject)

	server.SendInternalRequest(req, log, true)
	return nil
}
