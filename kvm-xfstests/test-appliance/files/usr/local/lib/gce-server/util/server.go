package util

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

const (
	SecretPath      = "/etc/lighttpd/server.pem"
	CertPath        = "/root/xfstests_bld/kvm-xfstests/.gce_xfstests_cert.pem"
	sessionsKeyPath = "/usr/local/lib/gce-server/.sessions_secret_key"
)

// UserOptions contains configs user sends to LTM or KCS.
type UserOptions struct {
	NoRegionShard bool   `json:"no_region_shard"`
	BucketSubdir  string `json:"bucket_subdir"`
	GsKernel      string `json:"gs_kernel"`
	ReportEmail   string `json:"report_email"`
	CommitID      string `json:"commit_id"`
	GitRepo       string `json:"git_repo"`
}

// LTMOptions contains configs LTM sends to KCS.
type LTMOptions struct {
	TestID string `json:"test_id"`
}

// LoginRequest contains a password for user authentication
type LoginRequest struct {
	Password string `json:"password"`
}

// UserRequest contains the full cmd from user in base 64 and some configs.
// LTM can send a UserRequest to KCS with an additional field ExtraOptions.
type UserRequest struct {
	CmdLine      string       `json:"orig_cmdline"`
	Options      *UserOptions `json:"options"`
	ExtraOptions *LTMOptions  `json:"extra_options"`
}

// SimpleResponse returns whether a web request succeeds.
type SimpleResponse struct {
	Status bool `json:"status"`
}

// BuildResponse returns the request status and gs path for the kernel image.
type BuildResponse struct {
	Status bool   `json:"status"`
	GSPath string `json:"gs_path"`
}

var (
	key   []byte
	store *sessions.CookieStore
)

func init() {
	if FileExists(sessionsKeyPath) {
		buf, err := ioutil.ReadFile(sessionsKeyPath)
		Check(err)
		key = buf
	} else {
		key = securecookie.GenerateRandomKey(32)
		err := ioutil.WriteFile(sessionsKeyPath, key, 0644)
		Check(err)
	}
	store = sessions.NewCookieStore(key)
}

// Index handles the root endpoint.
func Index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("index page"))
}

// Login handles the login reguest (not used for now).
func Login(w http.ResponseWriter, r *http.Request) {
	var c LoginRequest
	err := json.NewDecoder(r.Body).Decode(&c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	session, err := store.Get(r, "single-session")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	session.Values["pwd"] = c.Password
	err = session.Save(r, w)

	stat := SimpleResponse{true}
	js, err := json.Marshal(stat)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}
