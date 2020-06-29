package util

import (
	"encoding/json"
	"net/http"
)

// Options contains configs for web requests in the json POST body.
type Options struct {
	NoRegionShard bool   `json:"no_region_shard"`
	BucketSubdir  string `json:"bucket_subdir"`
	GsKernel      string `json:"gs_kernel"`
	ReportEmail   string `json:"report_email"`
	CommitID      string `json:"commit_id"`
	GitRepo       string `json:"git_repo"`
}

// UserRequest contains the full cmd from user in base 64 and some configs.
type UserRequest struct {
	CmdLine string  `json:"orig_cmdline"`
	Options Options `json:"options"`
}

// SimpleResponse returns whether a web request succeeds.
type SimpleResponse struct {
	Status bool `json:"status"`
}

// MsgResponse returns the request status and a string message.
type MsgResponse struct {
	Status bool   `json:"status"`
	Msg    string `json:"message"`
}

// Index handles the root endpoint.
func Index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("index page"))
}

// Login handles the login reguest (deprecated).
func Login(w http.ResponseWriter, r *http.Request) {
	stat := SimpleResponse{true}
	js, err := json.Marshal(stat)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}
