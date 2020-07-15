/*
Webserver endpoints for the gce-xfstests KCS (kernel compile server).

This stand-alone server handles requests to build a kernel image from the
client-side bash scripts or the LTM server.
The endpoints are:
	/login (deprecated) - to authenticate a user session, enforced by the flask
	webserver in the previoud implementation.

	/gce-xfstests - takes in a json POST in the form of LTMRequest, and runs the
	tests.

*/
package main

import (
	"encoding/json"
	"net/http"

	"gce-server/logging"
	"gce-server/server"
	"gce-server/util"

	"github.com/sirupsen/logrus"
)

// runCompile is the end point for a build request.
// Sends a simple status response back to requester.
func runCompile(w http.ResponseWriter, r *http.Request) {
	defer server.FailureResponse(w)

	log := server.Log.WithField("endpoint", "/gce-xfstests")

	var c server.TaskRequest
	err := json.NewDecoder(r.Body).Decode(&c)
	logging.CheckPanic(err, log, "Failed to parse json request")

	log.WithFields(logrus.Fields{
		"cmdLine":      c.CmdLine,
		"options":      c.Options,
		"extraOptions": c.ExtraOptions,
	}).Info("Received request")

	testID := util.GetTimeStamp()

	response := server.SimpleResponse{
		Status: true,
		TestID: testID,
	}

	if c.ExtraOptions == nil {
		log.WithField("testID", testID).Info("User request, generating testID")

		go StartBuild(c, testID)
		response.Msg = "Building kernel for user"

	} else {
		switch c.ExtraOptions.Requester {
		case server.LTMBuild:
			testID = c.ExtraOptions.TestID
			log.WithField("testID", testID).Info("LTM build request, use existing testID")

			go StartBuild(c, testID)
			response.TestID = testID
			response.Msg = "Building kernel for LTM"

		case server.LTMBisectStart:
			fallthrough
		case server.LTMBisectStep:
			testID = c.ExtraOptions.TestID
			log.WithField("testID", testID).Info("LTM bisect request, use existing testID")

			go RunBisect(c, testID)
			response.TestID = testID
			response.Msg = "Running git bisect task"
		default:
			response.Status = false
			response.Msg = "Unrecognized request"
		}
	}

	js, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

func main() {
	defer logging.CloseLog(server.Log)

	server.Log.Info("Launching KCS server")
	http.HandleFunc("/", server.Index)
	http.HandleFunc("/login", server.Login)
	http.HandleFunc("/gce-xfstests", runCompile)
	err := http.ListenAndServeTLS(":443", server.CertPath, server.SecretPath, nil)
	logging.CheckPanic(err, server.Log, "TLS server failed to launch")
}
