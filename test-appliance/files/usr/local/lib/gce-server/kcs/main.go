/*
Web server endpoints for the gce-xfstests KCS (kernel compile server).

This stand-alone server handles requests to build a kernel image from the
client-side bash scripts or the LTM server. It also supports auto git bisect.
The endpoints are:

	/login - authenticates a user session, implemented in server.go

	/gce-xfstests - takes in a json POST in the form of LTMRequest, and runs the
	tests.

	/internal - handles internal requests from LTM server.

	/internal-status - handles queries for running status from LTM server.
*/
package main

import (
	"net/http"

	"thunk.org/gce-server/util/check"
	"thunk.org/gce-server/util/mymath"
	"thunk.org/gce-server/util/server"

	"github.com/sirupsen/logrus"
)

// runCompile is the endpoint for a build request.
// Sends a simple status response back to requester.
func runCompile(w http.ResponseWriter, r *http.Request, serverLog *logrus.Entry, internal bool) {
	endpoint := "/gce-xfstests"
	if internal {
		endpoint = "/internal"
	}
	log := serverLog.WithField("endpoint", endpoint)

	c, err := server.ParseTaskRequest(w, r, internal)
	if !check.NoError(err, log, "Failed to parse request") {
		return
	}
	log.WithFields(logrus.Fields{
		"cmdLine":      c.CmdLine,
		"options":      c.Options,
		"extraOptions": c.ExtraOptions,
	}).Info("Received build request")

	if c.Options.ReportFailEmail == "" {
		c.Options.ReportFailEmail = c.Options.ReportEmail
	}

	response := server.SimpleResponse{
		Status: true,
	}

	if c.ExtraOptions == nil {
		testID := mymath.GetTimeStamp()
		log.WithField("testID", testID).Info("User request, generating testID")
		if err := check.ValidateTestID(testID); err != nil {
			log.WithField("testID", testID).WithError(err).Error("Invalid generated testID")
			_ = server.SendErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}

		go StartBuild(c, testID, serverLog)
		response.TestID = testID
		response.Msg = "Building kernel for user"

	} else {
		switch c.ExtraOptions.Requester {
		case server.LTMBuild:
			testID := c.ExtraOptions.TestID
			log.WithField("testID", testID).Info("LTM build request, use existing testID")
			if err := check.ValidateTestID(testID); err != nil {
				log.WithField("testID", testID).WithError(err).Error("Invalid testID in LTM build request")
				_ = server.SendErrorResponse(w, http.StatusBadRequest, err.Error())
				return
			}

			go StartBuild(c, testID, serverLog)
			response.TestID = testID
			response.Msg = "Building kernel for LTM"

		case server.LTMBisectStart:
			fallthrough
		case server.LTMBisectStep:
			testID := c.ExtraOptions.TestID
			log.WithField("testID", testID).Info("LTM bisect request, use existing testID")
			if err := check.ValidateTestID(testID); err != nil {
				log.WithField("testID", testID).WithError(err).Error("Invalid testID in LTM bisect request")
				_ = server.SendErrorResponse(w, http.StatusBadRequest, err.Error())
				return
			}

			go RunBisect(c, testID, serverLog)
			response.TestID = testID
			response.Msg = "Running git bisect task"
		default:
			response.Status = false
			response.Msg = "Unrecognized request"
		}
	}

	log.WithField("response", response).Info("Sending response")
	err = server.SendResponse(w, r, response)
	check.Panic(err, log, "Failed to send the response")
}

// status is the endpoint for querying running status.
// It only get exposed to LTM server.
func status(w http.ResponseWriter, r *http.Request, serverLog *logrus.Entry) {
	log := serverLog.WithField("endpoint", "/internal-status")
	c, err := server.ParseTaskRequest(w, r, true)
	if !check.NoError(err, log, "Failed to parse status request") {
		return
	}
	if c.ExtraOptions.Requester != server.Query {
		log.WithField("requester", c.ExtraOptions.Requester).Error("Unrecognized internal status request")
		_ = server.SendErrorResponse(w, http.StatusBadRequest, "Unrecognized request")
		return
	}
	log.Info("generating running status info")

	response := server.StatusResponse{
		Bisectors: BisectorStatus(),
	}
	log.WithField("response", response).Info("Sending response")

	err = server.SendResponse(w, r, response)
	check.Panic(err, log, "Failed to send the response")
}

func main() {
	s, err := server.New(":443", "kcs")
	if err != nil {
		panic(err)
	}

	s.Handler().Handle("/gce-xfstests", s.LoginHandler(s.FailureHandler(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			runCompile(w, r, s.Log(), false)
		})))).Methods("POST")
	s.Handler().Handle("/internal", s.FailureHandler(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			runCompile(w, r, s.Log(), true)
		}))).Methods("POST")
	s.Handler().Handle("/internal-status", s.FailureHandler(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			status(w, r, s.Log())
		}))).Methods("POST")

	stopTracker := make(chan struct{})
	finished := make(chan struct{})
	go StartTracker(s, stopTracker, finished)
	s.Start()
	close(stopTracker)
	<-finished
}
