package main

import (
	"time"

	"gce-server/util/check"
	"gce-server/util/email"
	"gce-server/util/gcp"
	"gce-server/util/logging"
	"gce-server/util/server"

	"github.com/sirupsen/logrus"
)

// Timeout defines the time out threshold for a kernel build
const Timeout = 1800

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

func waitKernel(gce *gcp.Service, prefix string, log *logrus.Entry) bool {
	log.Info("Waiting for kernel image")
	waitTime := 0

	for waitTime < Timeout {
		time.Sleep(60 * time.Second)
		waitTime += 60
		log.WithField("waited", waitTime).Debug("Keep waiting")

		if names, err := gce.GetFileNames(prefix); err == nil && len(names) > 0 {
			return true
		}
	}
	log.WithField("waited", waitTime).Panic("Failed to find kernel image in acceptable time")
	return false
}
