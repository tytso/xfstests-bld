package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"gce-server/logging"
	"gce-server/server"
	"gce-server/util"

	"github.com/sirupsen/logrus"
)

var buildLock sync.Mutex

// StartBuild starts a kernel build task.
// The kernel image is uploaded to gs bucket at path /kernels/bzImage-<testID>.
// If ExtraOptions is not nil, it rewrites gsKernel in original request and
// send it back to LTM to init a test.
func StartBuild(c server.TaskRequest, testID string) {
	log := server.Log.WithField("testID", testID)
	log.Info("Start building kernel")

	buildLog := logging.KCSLogDir + testID + ".log"
	subject := "xfstests KCS build failure " + testID
	defer util.ReportFailure(log, buildLog, c.Options.ReportEmail, subject)

	err := util.CreateDir(logging.KCSLogDir)
	logging.CheckPanic(err, log, "Failed to create dir")

	config, err := util.GetConfig(util.GceConfigFile)
	logging.CheckPanic(err, log, "Failed to get config")

	gsBucket := config.Get("GS_BUCKET")
	gsPath := fmt.Sprintf("gs://%s/kernels/bzImage-%s-onerun", gsBucket, testID)

	runBuild(c.Options.GitRepo, c.Options.CommitID, gsBucket, gsPath, testID, buildLog, log)

	if c.ExtraOptions != nil {
		c.Options.GsKernel = gsPath
		c.ExtraOptions.Requester = "kcs"
		sendRequest(c, log)
	}
}

func runBuild(url string, commit string, gsBucket string, gsPath string, testID string, buildLog string, log *logrus.Entry) {
	buildLock.Lock()
	defer buildLock.Unlock()

	file, err := os.Create(buildLog)
	logging.CheckPanic(err, log, "Failed to create file")
	defer file.Close()

	cmd := exec.Command(util.FetchBuildScript)
	cmdLog := log.WithFields(logrus.Fields{
		"GitRepo":  url,
		"commitID": commit,
		"gsBucket": gsBucket,
		"gsPath":   gsPath,
		"cmd":      cmd.String(),
	})
	cmdLog.Info("Start building kernel")

	env := map[string]string{
		"GIT_REPO":     url,
		"COMMIT":       commit,
		"GS_BUCKET":    gsBucket,
		"GS_PATH":      gsPath,
		"BUILD_KERNEL": "yes",
	}

	err = util.CheckRun(cmd, util.RootDir, env, file, file)
	file.Sync()
	logging.CheckPanic(err, cmdLog, "Failed to build kernel")
}

func launchLTM(log *logrus.Entry) {
	log.Info("Fetching LTM config file")

	cmd := exec.Command("gce-xfstests", "launch-ltm")
	cmdLog := log.WithField("cmd", cmd.String())
	w := cmdLog.Writer()
	defer w.Close()
	output, err := util.CheckOutput(cmd, util.RootDir, util.EmptyEnv, w)
	if err != nil && output != "The LTM instance already exists!\n" {
		cmdLog.WithField("output", output).WithError(err).Panic(
			"Failed to fetch LTM config file")
	}
}

// sendRequest sends a modified request back to LTM to init a test.
// LTM is assumed to be running, but needs to run launch-ltm once
// to generate .ltm_instance.
func sendRequest(c server.TaskRequest, log *logrus.Entry) {
	log.Info("Sending request to LTM")

	if !util.FileExists(util.LtmConfigFile) {
		launchLTM(log)
	}

	config, err := util.GetConfig(util.LtmConfigFile)
	logging.CheckPanic(err, log, "Failed to get LTM config")

	ip := config.Get("GCE_LTM_INT_IP")
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
		Timeout:   60 * time.Second,
	}

	resp, err := client.Do(req)
	logging.CheckPanic(err, log, "Failed to get response from LTM")

	defer resp.Body.Close()

	var c1 server.SimpleResponse

	err = json.NewDecoder(resp.Body).Decode(&c1)
	logging.CheckPanic(err, log, "Failed to parse json response")

	log.WithField("resp", c1).Debug("Received response from KCS")

	if !c1.Status {
		panic(c1.Msg)
	}
}
