package server

import (
	"encoding/json"
	"gce-server/logging"
	"gce-server/util"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
)

// file paths for certificates and keys
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
	BranchName    string `json:"branch_name"`
	UnWatch       bool   `json:"unwatch"`
}

// InternalOptions contains configs used by LTM and KCS internally
type InternalOptions struct {
	TestID    string `json:"test_id"`
	Requester string `json:"requester"`
	MockState string `json:"mock_state"`
}

// LoginRequest contains a password for user authentication
type LoginRequest struct {
	Password string `json:"password"`
}

// TaskRequest contains the full cmd from user in base 64 and some configs.
// LTM and KCS could add an additional field ExtraOptions when talks.
type TaskRequest struct {
	CmdLine      string           `json:"orig_cmdline"`
	Options      *UserOptions     `json:"options"`
	ExtraOptions *InternalOptions `json:"extra_options"`
}

// SimpleResponse returns whether a web request succeeds along with a message.
type SimpleResponse struct {
	Status bool   `json:"status"`
	TestID string `json:"testID"`
	Msg    string `json:"msg"`
}

var (
	key   []byte
	store *sessions.CookieStore
)

// Log logs all the server related messages.
// Initialized at the time of importing the server package.
var Log *logrus.Entry

func init() {
	Log = logging.InitLogger(logging.ServerLogPath)

	if util.FileExists(sessionsKeyPath) {
		buf, err := ioutil.ReadFile(sessionsKeyPath)
		logging.CheckPanic(err, Log, "Failed to read file")

		key = buf
	} else {
		key = securecookie.GenerateRandomKey(32)
		err := ioutil.WriteFile(sessionsKeyPath, key, 0644)
		logging.CheckPanic(err, Log, "Failed to write file")
	}
	store = sessions.NewCookieStore(key)
}

// Index handles the root endpoint.
func Index(w http.ResponseWriter, r *http.Request) {
	log := Log.WithField("endpoint", "/")
	log.Info("Request received, returning index contents")
	log.Info("Returning index contents")
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("index page"))
}

// Login handles the login reguest (not used for now).
func Login(w http.ResponseWriter, r *http.Request) {
	log := Log.WithField("endpoint", "/login")
	log.Info("Request received")
	var c LoginRequest
	err := json.NewDecoder(r.Body).Decode(&c)
	if !logging.CheckNoError(err, log, "Failed to parse json request") {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	session, err := store.Get(r, "single-session")
	if !logging.CheckNoError(err, log, "Failed to retrieve user session") {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// TODO: implement password validation
	session.Values["pwd"] = c.Password
	err = session.Save(r, w)
	if !logging.CheckNoError(err, log, "Failed to save user session") {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stat := SimpleResponse{
		Status: true,
		Msg:    "login succeeded",
	}
	log.WithField("response", stat).Info("Login succeeded")

	js, _ := json.Marshal(stat)
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// FailureResponse handles panic and send error back to client
func FailureResponse(w http.ResponseWriter) {
	log := Log.WithField("endpoint", "failure")

	if r := recover(); r != nil {
		log.WithField("panic", r).Warn("Failed to handle request")
		msg := "unknown panic"
		switch s := r.(type) {
		case string:
			msg = s
		case error:
			msg = s.Error()
		case *logrus.Entry:
			msg = s.Message
		}

		response := SimpleResponse{
			Status: false,
			Msg:    msg,
		}
		log.WithField("response", response).Info("Sending back error message")
		js, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
}
