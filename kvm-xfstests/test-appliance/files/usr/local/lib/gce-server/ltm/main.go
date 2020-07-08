package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"example.com/gce-server/util"
)

const (
	ServerLogPath = "/var/log/lgtm/lgtm.log"
	TestLogPath   = "/var/log"
	SecretPath    = "/etc/lighttpd/server.pem"
	CertPath      = "/root/xfstests_bld/kvm-xfstests/.gce_xfstests_cert.pem"
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

type LTMRespond struct {
	Status bool `json:"status"`
}

func index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("LTM test page"))
	log.Println("Hello World")
}

func login(w http.ResponseWriter, r *http.Request) {
	stat := LTMRespond{true}
	js, err := json.Marshal(stat)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
	log.Println("log in here", string(js))
}

func runTests(w http.ResponseWriter, r *http.Request) {
	var c LTMRequest
	err := json.NewDecoder(r.Body).Decode(&c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, _ := base64.StdEncoding.DecodeString(c.CmdLine)
	log.Printf("receive test request: %+v\n%s", c.Options, string(data))

	status := LTMRespond{true}
	js, _ := json.Marshal(status)
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
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

func main() {
	http.HandleFunc("/", index)
	http.HandleFunc("/login", login)
	http.HandleFunc("/gce-xfstests", runTests)
	err := http.ListenAndServeTLS(":443", CertPath, SecretPath, nil)
	if err != nil {
		log.Fatal("ListenandServer: ", err)
	}
}
