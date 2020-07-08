package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"

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
	Status bool   `json:"status"`
	Msg    string `json:"message"`
}

func index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("KCS test page"))
	log.Println("Hello World")
}

func login(w http.ResponseWriter, r *http.Request) {
	stat := LTMRespond{true, ""}
	js, err := json.Marshal(stat)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
	log.Println("log in here", string(js))
}

func runCompile(w http.ResponseWriter, r *http.Request) {
	var c LTMRequest
	err := json.NewDecoder(r.Body).Decode(&c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := base64.StdEncoding.DecodeString(c.CmdLine)
	util.Check(err)
	c.CmdLine = string(data)
	log.Printf("receive test request: %+v\n%s", c.Options, string(data))

	status := buildKernel(c)

	js, err := json.Marshal(status)
	util.Check(err)
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

func main() {
	http.HandleFunc("/", index)
	http.HandleFunc("/login", login)
	http.HandleFunc("/gce-xfstests", runCompile)
	err := http.ListenAndServeTLS(":443", CertPath, SecretPath, nil)
	util.Check(err)
}
