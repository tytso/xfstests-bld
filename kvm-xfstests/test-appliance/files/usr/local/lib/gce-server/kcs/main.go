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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"gce-server/logging"
	"gce-server/server"

	"github.com/sirupsen/logrus"
)

/*
runCompile is the end point for launching a kernel compile task.

*/
func runCompile(w http.ResponseWriter, r *http.Request) {
	log := server.Log.WithField("endpoint", "/gce-xfstests")
	log.Info("Request received")

	defer server.FailureResponse(w)

	var c server.UserRequest
	err := json.NewDecoder(r.Body).Decode(&c)
	logging.CheckPanic(err, log, "Failed to parse json request")

	data, err := base64.StdEncoding.DecodeString(c.CmdLine)
	logging.CheckPanic(err, log, "Failed to decode cmdline")

	c.CmdLine = string(data)
	log.WithFields(logrus.Fields{
		"cmdline":      c.CmdLine,
		"options":      fmt.Sprintf("%+v", c.Options),
		"ExtraOptions": fmt.Sprintf("%+v", c.ExtraOptions),
	}).Info("Receive compile request")

	gsPath, err := StartBuild(c)
	logging.CheckPanic(err, log, "Failed to start builder")

	response := server.SimpleResponse{
		Status: true,
		Msg:    gsPath,
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
