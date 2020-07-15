package server

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"gce-server/logging"
	"gce-server/util"
	"io/ioutil"
	"net/http"
	"os/exec"
	"time"

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

// RequestType specifies the type of a json request
type RequestType int

const (
	// Unspecified indicates RequestType is not set in the json request.
	Unspecified RequestType = iota
	// LTMBuild indicates a build request from LTM to KCS.
	LTMBuild
	// LTMBisectStart indicates a bisect start request from LTM to KCS.
	LTMBisectStart
	// LTMBisectStep indicates a bisect step request from LTM to KCS.
	LTMBisectStep
	// KCSTest indicates a test request from KCS to LTM.
	KCSTest
	// KCSBisectStep indicates a bisect step request from KCS to LTM.
	KCSBisectStep
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
	BadCommit     string `json:"bad_commit"`
	GoodCommit    string `json:"good_commit"`
}

// InternalOptions contains configs used by LTM and KCS internally
type InternalOptions struct {
	TestID     string      `json:"test_id"`
	Requester  RequestType `json:"requester"`
	TestResult bool        `json:"test_result"`
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

// SendInternalRequest sends a task request between LTM and KCS.
// The request is from LTM to KCS if toKCS is true.
func SendInternalRequest(c TaskRequest, log *logrus.Entry, toKCS bool) {
	receiver := "KCS"
	if !toKCS {
		receiver = "LTM"
	}
	log.Info("Sending request to " + receiver)

	var ip string
	if toKCS {
		launchKCS(log)
		config, err := util.GetConfig(util.KcsConfigFile)
		logging.CheckPanic(err, log, "Failed to get KCS config")
		ip = config.Get("GCE_KCS_INT_IP")

		// pwd := config.Get("GCE_KCS_PWD")
		// TODO: add login step

	} else {
		if !util.FileExists(util.LtmConfigFile) {
			launchLTM(log)
		}
		config, err := util.GetConfig(util.LtmConfigFile)
		logging.CheckPanic(err, log, "Failed to get LTM config")
		ip = config.Get("GCE_LTM_INT_IP")
	}

	url := fmt.Sprintf("https://%s/gce-xfstests", ip)

	js, _ := json.Marshal(c)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(js))
	logging.CheckPanic(err, log.WithField("js", js), "Failed to format request")

	req.Header.Set("Content-Type", "application/json")

	cert, err := tls.LoadX509KeyPair(CertPath, SecretPath)
	logging.CheckPanic(err, log, "Failed to load key pair")

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	}
	tlsConfig.BuildNameToCertificate()
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	client := &http.Client{
		Transport: transport,
	}
	if toKCS {
		client.Timeout = 20 * time.Second
	} else {
		client.Timeout = 60 * time.Second
	}

	var resp *http.Response
	attempts := 5
	for attempts > 0 {
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		attempts--
		log.WithError(err).WithField("attemptsLeft", attempts).Debug("Failed to connect to " + receiver)
		time.Sleep(10 * time.Second)
	}
	logging.CheckPanic(err, log, "Failed to get response from "+receiver)

	defer resp.Body.Close()

	var c1 SimpleResponse

	err = json.NewDecoder(resp.Body).Decode(&c1)
	logging.CheckPanic(err, log, "Failed to parse json response")

	log.WithField("resp", c1).Debug("Received response from " + receiver)

	if !c1.Status {
		log.Panic(c1.Msg)
	}
}

// launchKCS attempts to launch the KCS. If the exit status is 1
// due to kcs already exists, no panic is thrown.
func launchKCS(log *logrus.Entry) {
	log.Info("Launching KCS server")

	cmd := exec.Command("gce-xfstests", "launch-kcs")
	cmdLog := log.WithField("cmd", cmd.String())
	w := cmdLog.Writer()
	defer w.Close()
	output, err := util.CheckOutput(cmd, util.RootDir, util.EmptyEnv, w)
	if err != nil && output != "The KCS instance already exists!\n" {
		cmdLog.WithField("output", output).WithError(err).Panic("Failed to launch KCS")
	}
}

// launchLTM attempts to launch the LTM. Usually used to generate .ltm_instance
// since LTM should always be running.
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
