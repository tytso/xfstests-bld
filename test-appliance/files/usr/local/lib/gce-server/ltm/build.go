package main

import (
	"thunk.org/gce-server/util/check"
	"thunk.org/gce-server/util/email"
	"thunk.org/gce-server/util/logging"
	"thunk.org/gce-server/util/server"
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
	defer email.ReportFailure(log, logFile, req.Options.ReportFailEmail, subject)

	server.SendInternalRequest(req, log, true)
}
