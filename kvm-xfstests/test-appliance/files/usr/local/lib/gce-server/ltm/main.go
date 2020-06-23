package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"example.com/gce-server/util"
	"google.golang.org/api/compute/v1"
)

const (
	ServerLogPath = "/var/log/lgtm/lgtm.log"
	TestLogDir    = "/var/log/lgtm/ltm_logs/"
	SecretPath    = "/etc/lighttpd/server.pem"
	CertPath      = "/root/xfstests_bld/kvm-xfstests/.gce_xfstests_cert.pem"
	LTMUserName   = "ltm"
)

type Options struct {
	NoRegionShard bool   `json:"no_region_shard"`
	BucketSubdir  string `json:"bucket_subdir"`
	GsKernel      string `json:"gs_kernel"`
	ReportEmail   string `json:"report_email"`
	CommitID      string `json:"commit_id"`
	GitRepo       string `json:"git_repo"`
}

type LTMRequest struct {
	CmdLine string  `json:"orig_cmdline"`
	Options Options `json:"options"`
}

type LTMResponse struct {
	Status bool `json:"status"`
}

type TestResponse struct {
	Status bool        `json:"status"`
	Info   SharderInfo `json:"info"`
}

func index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("LTM test page"))
	log.Println("Hello World")
}

func login(w http.ResponseWriter, r *http.Request) {
	stat := LTMResponse{true}
	js, err := json.Marshal(stat)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
	log.Println("received login request", string(js))
}

// end point for launching a gce-xfstests test run
// orig_cmdline is expected in the request content
func runTests(w http.ResponseWriter, r *http.Request) {
	var c LTMRequest
	err := json.NewDecoder(r.Body).Decode(&c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := base64.StdEncoding.DecodeString(c.CmdLine)
	util.Check(err)
	c.CmdLine = string(data)
	log.Printf("receive test request: %+v\n", &c)

	tester := NewTestManager(c)
	log.Printf("create test manager: %+v", &tester)
	sharderInfo := tester.Run()

	response := TestResponse{
		Status: true,
		Info:   sharderInfo,
	}
	js, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

func main() {
	http.HandleFunc("/", index)
	http.HandleFunc("/login", login)
	http.HandleFunc("/gce-xfstests", runTests)
	err := http.ListenAndServeTLS(":443", CertPath, SecretPath, nil)
	if err != nil {
		log.Fatal("ListenandServer: ", err)
	}
}

var repo util.Repository

func test() {
	reader := bufio.NewReader(os.Stdin)
	for true {
		arg, _ := reader.ReadString('\n')
		switch arg[:len(arg)-1] {
		case "clone":
			repo = util.Clone("https://github.com/XiaoyangShen/spinner_test.git", "master")
		case "commit":
			id := repo.GetCommit()
			log.Println(id)
		case "pull":
			repo.Pull()
		case "watch":
			repo.Watch()
		}
	}
}

func test1() {
	reader := bufio.NewReader(os.Stdin)
	for true {
		arg, _ := reader.ReadString('\n')

		validArg, configs := util.ParseCmd(arg[:len(arg)-1])
		log.Printf("%s; %+v\n", validArg, configs)
	}
}

func test2() {
	gce := util.NewGceService()
	info, err := gce.GetInstanceInfo("gce-xfstests-bldsrv", "us-central1-f", "xfstests-ltm")
	util.Check(err)
	log.Printf("%+v", info.Metadata)
	for _, item := range info.Metadata.Items {
		log.Printf("%+v", item)
	}

	val := "ahaah"
	newMetadata := compute.Metadata{
		Fingerprint: info.Metadata.Fingerprint,
		Items: []*compute.MetadataItems{
			{
				Key:   "shutdown_reason",
				Value: &val,
			},
		},
	}
	gce.SetMetadata("gce-xfstests-bldsrv", "us-central1-f", "xfstests-ltm", &newMetadata)
}
