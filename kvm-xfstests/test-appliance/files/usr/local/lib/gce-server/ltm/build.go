package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"gce-server/logging"
	"gce-server/server"
	"gce-server/util"

	"github.com/sirupsen/logrus"
)

// Timeout defines the time out threshold for a kernel build
const Timeout = 1800

// StartBuild inits a build task by calling KCS to build a kernel image.
// Sends failure report email to user on failures
func StartBuild(req server.TaskRequest, testID string) {
	logDir := logging.LTMLogDir + testID + "/"
	err := util.CreateDir(logDir)
	if err != nil {
		panic(err)
	}
	logFile := logDir + "run.log"
	log := logging.InitLogger(logFile)
	defer logging.CloseLog(log)

	subject := "xfstests LTM build request failure " + testID
	defer util.ReportFailure(log, logFile, req.Options.ReportEmail, subject)

	args := server.InternalOptions{
		TestID:    testID,
		Requester: "ltm",
	}
	req.ExtraOptions = &args

	resp := sendRequest(req, log)
	defer resp.Body.Close()

	var c server.SimpleResponse

	err = json.NewDecoder(resp.Body).Decode(&c)
	logging.CheckPanic(err, log, "Failed to parse json response")

	log.WithField("resp", c).Debug("Received response from KCS")

	if !c.Status {
		panic(c.Msg)
	}
}

// launchKCS attempts to launch the KCS. If the exit status is 1
// due to kcs already exists, no panic is thrown.
func launchKCS(log *logrus.Entry) {
	log.Info("Launching KCS server")

	cmd := exec.Command("gce-xfstests", "launch-kcs")
	cmdLog := log.WithField("cmd", cmd.String())
	w := cmdLog.Writer()
	defer w.Close()
	output, err := util.CheckOutput(cmd, util.RootDir, util.EmptyEnv, w)
	if err != nil && output != "The KCS instance already exists!\n" {
		cmdLog.WithField("output", output).WithError(err).Panic("Failed to launch KCS")
	}
}

func sendRequest(c server.TaskRequest, log *logrus.Entry) *http.Response {
	log.Info("Sending request to KCS")

	launchKCS(log)

	config, err := util.GetConfig(util.KcsConfigFile)
	logging.CheckPanic(err, log, "Failed to get KCS config")

	ip := config.Get("GCE_KCS_INT_IP")
	// pwd := config.Get("GCE_KCS_PWD")
	// TODO: add login step

	url := fmt.Sprintf("https://%s/gce-xfstests", ip)

	js, _ := json.Marshal(c)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(js))
	logging.CheckPanic(err, log.WithField("js", js), "Failed to format request")

	req.Header.Set("Content-Type", "application/json")

	cert, err := tls.LoadX509KeyPair(server.CertPath, server.SecretPath)
	logging.CheckPanic(err, log, "Failed to load key pair")

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	}
	tlsConfig.BuildNameToCertificate()
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	var resp *http.Response
	attempts := 10
	for attempts > 0 {
		resp, err = client.Do(req)
		if err == nil {
			return resp
		}
		attempts--
		log.WithError(err).WithField("attemptsLeft", attempts).Debug("Failed to connect to KCS")
		time.Sleep(10 * time.Second)
	}
	logging.CheckPanic(err, log, "Failed to get response from KCS")
	return nil
}

func waitKernel(gce *util.GceService, prefix string, log *logrus.Entry) bool {
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
