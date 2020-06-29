package util

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

const sessionsKeyPath = "/usr/local/lib/gce-server/.sessions_secret_key"

// Options contains configs for web requests in the json POST body.
type Options struct {
	NoRegionShard bool   `json:"no_region_shard"`
	BucketSubdir  string `json:"bucket_subdir"`
	GsKernel      string `json:"gs_kernel"`
	ReportEmail   string `json:"report_email"`
	CommitID      string `json:"commit_id"`
	GitRepo       string `json:"git_repo"`
}

// LoginRequest contains a password for user authentication
type LoginRequest struct {
	Password string `json:"password"`
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
