package server

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"runtime/debug"
	"time"

	"gce-server/util/check"
	"gce-server/util/gcp"
	"gce-server/util/logging"

	"github.com/gorilla/mux"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
)

// file paths for certificates and keys
const (
	certPath        = "/root/xfstests_bld/kvm-xfstests/.gce_xfstests_cert.pem"
	secretPath      = "/etc/lighttpd/server.pem"
	sessionsKeyPath = "/usr/local/lib/gce-server/.sessions_secret_key"
)

// RequestType defines the type of a json request.
type RequestType int

const (
	// DefaultRequest indicates RequestType is not set in the json request.
	DefaultRequest RequestType = iota
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

// ResultType defines the result state of a test.
type ResultType int

const (
	// DefaultResult indicates ResultType is not set in the json request.
	DefaultResult ResultType = iota
	// Pass indicates all test passed.
	Pass
	// Failure indicates at least one failed test.
	Failure
	// UnknownResult indicates something unexpected happened so skip this commit.
	UnknownResult
)

const (
	kcsTimeout    = 30 * time.Second
	ltmTimeout    = 60 * time.Second
	checkInterval = 10 * time.Second
	maxAttempts   = 5
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
	TestResult ResultType  `json:"test_result"`
	Password   string      `json:"password"`
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

// Instance implements an https server with encrypted sessions and log.
type Instance struct {
	httpServer *http.Server
	addr       string
	router     *mux.Router

	store *sessions.CookieStore
	log   *logrus.Entry
}

var (
	key      []byte
	password string
)

func init() {
	if check.FileExists(sessionsKeyPath) {
		buf, err := ioutil.ReadFile(sessionsKeyPath)
		if err != nil {
			panic("Failed to read session key file")
		}

		key = buf
	} else {
		key = securecookie.GenerateRandomKey(32)
		err := ioutil.WriteFile(sessionsKeyPath, key, 0644)
		if err != nil {
			panic("Failed to write session key file")
		}
	}

	var err error
	if gcp.LtmConfig != nil {
		password, err = gcp.LtmConfig.Get("GCE_LTM_PWD")

	} else if gcp.KcsConfig != nil {
		password, err = gcp.KcsConfig.Get("GCE_KCS_PWD")
	} else {
		panic("Failed to find config file")
	}
	if err != nil {
		panic(err)
	}

}

// New sets up a new https server.
func New(addr string) (*Instance, error) {
	log := logging.InitLogger(logging.ServerLogPath)
	log.Info("Initiating server")

	server := &Instance{
		addr:   addr,
		router: mux.NewRouter(),
		store:  sessions.NewCookieStore(key),
		log:    log,
	}

	server.router.HandleFunc("/", server.Index).Methods("GET")
	server.router.HandleFunc("/login", server.Login).Methods("POST")

	return server, nil
}

// Handler returns the handler for the https server.
func (server *Instance) Handler() *mux.Router {
	return server.router
}

// Log returns the log for the https server.
func (server *Instance) Log() *logrus.Entry {
	return server.log
}

// Start launches the server. Custom endpoints shuold be registered already.
func (server *Instance) Start() {
	server.log.Info("Launching server")

	server.httpServer = &http.Server{
		Addr:    server.addr,
		Handler: server.router,
	}
	err := server.httpServer.ListenAndServeTLS(certPath, secretPath)

	if err != http.ErrServerClosed {
		server.log.WithError(err).Error("Server stopped unexpectedly")
		server.Shutdown()
	} else {
		server.log.WithError(err).Info("Server stopped")
	}
	logging.CloseLog(server.log)
}

// Shutdown shut down the web service and delete the VM instance.
func (server *Instance) Shutdown() {

}

// Index handles the root endpoint.
func (server *Instance) Index(w http.ResponseWriter, r *http.Request) {
	log := server.log.WithField("endpoint", "/")
	log.Info("Request received, returning index contents")
	log.Info("Returning index contents")
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("index page"))
}

// Login handles the login reguest (not used for now).
func (server *Instance) Login(w http.ResponseWriter, r *http.Request) {
	log := server.log.WithField("endpoint", "/login")
	log.Info("Request received")
	var c LoginRequest
	err := json.NewDecoder(r.Body).Decode(&c)
	if !check.NoError(err, log, "Failed to parse json request") {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	session, err := server.store.Get(r, "single-session")
	if !check.NoError(err, log, "Failed to retrieve user session") {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if c.Password != password {
		log.Error("Wrong password")
		http.Error(w, "Wrong password", http.StatusBadRequest)
		return
	}

	session.Values["pwd"] = c.Password
	err = session.Save(r, w)
	if !check.NoError(err, log, "Failed to save user session") {
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

// LoginHandler validates the user session and passes over to the next handler.
func (server *Instance) LoginHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := server.log.WithField("endpoint", "UserLoginHandler")

		session, err := server.store.Get(r, "single-session")
		if !check.NoError(err, log, "Failed to retrieve user session") {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if pwd, ok := session.Values["pwd"].(string); !ok || pwd != password {
			log.Error("password validation failed")
			http.Error(w, "Login failed", http.StatusForbidden)
			return
		}

		log.Info("password validation succeeded")
		next.ServeHTTP(w, r)
	})
}

// FailureHandler handles panic and send error back to client
func (server *Instance) FailureHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			log := server.log.WithField("endpoint", "FailureHandler")

			if re := recover(); re != nil {
				log.Error("Failed to handle request, get stack trace")
				log.Error(string(debug.Stack()))
				msg := "unknown panic"
				switch s := re.(type) {
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
		}()

		next.ServeHTTP(w, r)
	})
}

// ParseTaskRequest parses the request into a TaskRequest struct
// Validates the password if the request is internal
func ParseTaskRequest(w http.ResponseWriter, r *http.Request) (TaskRequest, error) {
	var c TaskRequest
	err := json.NewDecoder(r.Body).Decode(&c)
	if err != nil {
		http.Error(w, "Failed to parse json request", http.StatusInternalServerError)
		return c, err
	}

	if c.ExtraOptions != nil && c.ExtraOptions.Password != password {
		http.Error(w, "Login failed", http.StatusForbidden)
		return c, fmt.Errorf("Failed to validate password")
	}

	return c, nil
}

// SendResponse sends a formatted response back to the requester.
func SendResponse(w http.ResponseWriter, r *http.Request, resp SimpleResponse) error {
	js, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(js)
	if err != nil {
		return err
	}

	return nil
}

// SendInternalRequest sends a task request between LTM and KCS.
// The request is from LTM to KCS if toKCS is true.
// Append a password field for internal request validation.
func SendInternalRequest(c TaskRequest, log *logrus.Entry, toKCS bool) {
	receiver := "KCS"
	if !toKCS {
		receiver = "LTM"
	}
	log.Info("Sending request to " + receiver)

	var config *gcp.Config
	if toKCS {
		launchKCS(log)
		gcp.Update()
		config = gcp.KcsConfig

	} else {
		launchLTM(log)
		gcp.Update()
		config = gcp.LtmConfig
	}
	if config == nil {
		log.Panicf("Failed to get %s config", receiver)
	}
	ip, err := config.Get("GCE_" + receiver + "_INT_IP")
	check.Panic(err, log, "Failed to get ip for "+receiver)

	url := fmt.Sprintf("https://%s/internal", ip)

	if c.ExtraOptions == nil {
		log.Panic("No internal option fields set in the request")
	}
	c.ExtraOptions.Password = password

	js, err := json.Marshal(c)
	check.Panic(err, log, "Failed to encode json request body")
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(js))
	check.Panic(err, log.WithField("js", js), "Failed to format request")

	req.Header.Set("Content-Type", "application/json")

	cert, err := tls.LoadX509KeyPair(certPath, secretPath)
	check.Panic(err, log, "Failed to load key pair")

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
		client.Timeout = kcsTimeout
	} else {
		client.Timeout = ltmTimeout
	}

	var resp *http.Response
	for attempts := maxAttempts; attempts > 0; attempts-- {
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		log.WithError(err).WithField("attemptsLeft", attempts).Debug("Failed to connect to " + receiver)
		time.Sleep(checkInterval)
	}
	check.Panic(err, log, "Failed to get response from "+receiver)

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, err := ioutil.ReadAll(resp.Body)
		check.Panic(err, log, "Failed to read response body")
		log.WithField("resp", string(b)).Panic("Response status is not OK")
	}

	var c1 SimpleResponse

	err = json.NewDecoder(resp.Body).Decode(&c1)
	check.Panic(err, log, "Failed to parse json response")

	log.WithField("resp", c1).Debug("Received response from " + receiver)

	if !c1.Status {
		log.WithField("msg", c1.Msg).Panic("Request failed with message from " + receiver)
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
	output, err := check.Output(cmd, check.RootDir, check.EmptyEnv, w)
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
	output, err := check.Output(cmd, check.RootDir, check.EmptyEnv, w)
	if err != nil && output != "The LTM instance already exists!\n" {
		cmdLog.WithField("output", output).WithError(err).Panic(
			"Failed to fetch LTM config file")
	}
}
