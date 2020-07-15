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
	}).Info("Received compile request")

	testID := util.GetTimeStamp()
	if c.ExtraOptions == nil {
		log.WithField("testID", testID).Info("User request, generating testID")
	} else if logging.MOCK {
		testID = c.ExtraOptions.TestID
		log.WithField("testID", testID).Info("Mock build")
		go MockStartBuild(c, testID)
	} else if c.ExtraOptions.Requester == "ltm" {
		testID = c.ExtraOptions.TestID
		log.WithField("testID", testID).Info("LTM request, use existing testID")
		go StartBuild(c, testID)
	}

	response := server.SimpleResponse{
		Status: true,
		TestID: testID,
		Msg:    "Building kernel",
	}

	log.WithField("response", response).Info("Kernel builder started")
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
