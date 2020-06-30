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
	"log"
	"net/http"

	"gce-server/util"
)

/*
runCompile is the end point for launching a kernel compile task.

*/
func runCompile(w http.ResponseWriter, r *http.Request) {
	var c util.UserRequest
	err := json.NewDecoder(r.Body).Decode(&c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := base64.StdEncoding.DecodeString(c.CmdLine)
	util.Check(err)
	c.CmdLine = string(data)
	log.Printf("receive compile request: %+v\n", c)

	gsPath := StartBuild(c)
	respond := util.BuildResponse{
		Status: true,
		GSPath: gsPath,
	}

	js, err := json.Marshal(respond)
	util.Check(err)
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

func main() {
	log.Printf("launching KCS server")
	http.HandleFunc("/", util.Index)
	http.HandleFunc("/login", util.Login)
	http.HandleFunc("/gce-xfstests", runCompile)
	err := http.ListenAndServeTLS(":443", util.CertPath, util.SecretPath, nil)
	util.Check(err)
}
