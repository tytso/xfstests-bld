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

// RunBuild launches the KCS server to build a kernel image.
func RunBuild(gitRepo string, commitID string, reportEmail string, testID string, gce *util.GceService, sharderLog *logrus.Entry) {
	log := sharderLog.WithFields(logrus.Fields{
		"gitRepo":  gitRepo,
		"commitID": commitID,
	})
	prefix := fmt.Sprintf("kernels/bzImage-%s", testID)
	names, err := gce.GetFileNames(prefix)
	logging.CheckPanic(err, log, "Failed to search for files")

	if len(names) > 0 {
		log.WithField("filename", names[0]).Info("kernel image file already exists on gs, skip building")
		return
	}

	launchKCS(log)

	resp := sendRequest(gitRepo, commitID, reportEmail, testID, log)
	defer resp.Body.Close()

	var c server.SimpleResponse

	err = json.NewDecoder(resp.Body).Decode(&c)
	logging.CheckPanic(err, log, "Failed to parse json response")

	log.WithField("rep", c).Debug("Received response from KCS")

	waitKernel(gce, prefix, log)
}

func launchKCS(log *logrus.Entry) {
	log.Info("Launching KCS server")

	cmd := exec.Command("gce-xfstests", "launch-kcs")
	cmdLog := log.WithField("cmd", cmd.String())
	w := cmdLog.Writer()
	defer w.Close()
	// exit status is 1 if kcs already exists
	output, err := util.CheckOutput(cmd, util.RootDir, util.EmptyEnv, w)
	if err != nil && output != "The KCS instance already exists!\n" {
		cmdLog.WithField("output", output).WithError(err).Panic("Failed to launch KCS")
	}
}

func sendRequest(gitRepo string, commitID string, reportEmail string, testID string, log *logrus.Entry) *http.Response {

	config, err := util.GetConfig(util.KcsConfigFile)
	logging.CheckPanic(err, log, "Failed to get kcs config")

	ip := config.Get("GCE_KCS_INT_IP")
	// pwd := config.Get("GCE_KCS_PWD")
	// TODO: add login step

	url := fmt.Sprintf("https://%s/gce-xfstests", ip)

	args1 := server.UserOptions{
		GitRepo:     gitRepo,
		CommitID:    commitID,
		ReportEmail: reportEmail,
	}
	args2 := server.LTMOptions{
		TestID: testID,
	}
	request := server.UserRequest{
		Options:      &args1,
		ExtraOptions: &args2,
	}

	js, _ := json.Marshal(request)
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
	logging.CheckPanic(err, log, "Failed to talk to KCS in acceptable time")
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
