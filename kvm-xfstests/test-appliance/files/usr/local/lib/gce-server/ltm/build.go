package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"gce-server/util"
)

// RunBuild launches the KCS server to build a kernel image.
func RunBuild(gitRepo string, commitID string, testID string, gce *util.GceService) {
	prefix := fmt.Sprintf("kernels/bzImage-%s", testID)
	if names := gce.GetFileNames(prefix); len(names) > 0 {
		log.Printf("kernel image file already exists on gs, skip building")
		return
	}

	launchKCS()

	resp, err := sendRequest(gitRepo, commitID, testID)
	util.Check(err)

	defer resp.Body.Close()

	var c util.BuildResponse

	err = json.NewDecoder(resp.Body).Decode(&c)
	util.Check(err)
	log.Printf("%+v", c)

	status := waitKernel(gce, prefix)
	if !status {
		log.Fatal("wait for KCS build timed out")
	}
}

func launchKCS() {
	log.Printf("launching KCS server")
	cmd := exec.Command("gce-xfstests", "launch-kcs")
	// exit status 1 if kcs already exists
	output, err := util.CheckOutput(cmd, util.RootDir, util.EmptyEnv, os.Stderr)
	if err != nil && output != "The KCS instance already exists!\n" {
		log.Printf(output)
		log.Fatal(err)
	}
}

func sendRequest(gitRepo string, commitID string, testID string) (*http.Response, error) {

	config := util.GetConfig(util.KcsConfigFile)
	ip := config.Get("GCE_KCS_INT_IP")
	// pwd := config.Get("GCE_KCS_PWD")
	url := fmt.Sprintf("https://%s/gce-xfstests", ip)

	args1 := util.UserOptions{
		GitRepo:  gitRepo,
		CommitID: commitID,
	}
	args2 := util.LTMOptions{
		TestID: testID,
	}
	request := util.UserRequest{
		Options:      &args1,
		ExtraOptions: &args2,
	}

	js, _ := json.Marshal(request)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(js))
	util.Check(err)
	req.Header.Set("Content-Type", "application/json")

	cert, err := tls.LoadX509KeyPair(util.CertPath, util.SecretPath)
	util.Check(err)

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
	attempts := 5
	for attempts > 0 {
		resp, err = client.Do(req)
		if err == nil {
			return resp, err
		}
		attempts--
		time.Sleep(10 * time.Second)
		log.Printf(err.Error())
	}
	return resp, err

}

func waitKernel(gce *util.GceService, prefix string) bool {
	waitTime := 0

	for true {
		time.Sleep(60 * time.Second)
		waitTime += 60
		log.Printf("wait time: %d", waitTime)

		if names := gce.GetFileNames(prefix); len(names) > 0 {
			return true
		} else if waitTime > 1800 {
			break
		}
	}
	return false
}
