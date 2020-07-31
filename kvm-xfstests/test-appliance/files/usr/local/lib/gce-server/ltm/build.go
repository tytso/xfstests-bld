package main

import (
	"gce-server/util/check"
	"gce-server/util/email"
	"gce-server/util/logging"
	"gce-server/util/server"
)

// ForwardKCS forwards a build or git bisect start task to KCS.
// Sends failure report email to user on failures
func ForwardKCS(req server.TaskRequest, testID string) {
	logDir := logging.LTMLogDir + testID + "/"
	err := check.CreateDir(logDir)
	if err != nil {
		panic(err)
	}
	logFile := logDir + "run.log"
	log := logging.InitLogger(logFile)
	defer logging.CloseLog(log)

	subject := "xfstests LTM forwarding request failure " + testID
	defer email.ReportFailure(log, logFile, req.Options.ReportEmail, subject)

	server.SendInternalRequest(req, log, true)
}
