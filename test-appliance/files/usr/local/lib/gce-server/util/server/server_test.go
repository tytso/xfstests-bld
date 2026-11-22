package server

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewInitializesHTTPServerAndConcurrentShutdown(t *testing.T) {
	// Zero-value Instance guard must not panic when httpServer is nil.
	var zero Instance
	zero.Shutdown()
	zero.Close()

	inst, err := New("127.0.0.1:0", "test")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer inst.Close()

	if inst.httpServer == nil {
		t.Fatal("expected New() to initialize httpServer")
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inst.Shutdown()
		}()
	}
	wg.Wait()
}

func TestIsAlreadyExistsOutput(t *testing.T) {
	kcsOut := "Removing old cookies...\nStarting KCS instance...\nThe KCS instance already exists!\n"
	if !isAlreadyExistsOutput(kcsOut, "The KCS instance already exists!") {
		t.Fatal("expected multi-line launch-kcs output to match already-exists check")
	}

	ltmOut := "Checking config...\nThe LTM instance already exists!\nDone.\n"
	if !isAlreadyExistsOutput(ltmOut, "The LTM instance already exists!") {
		t.Fatal("expected multi-line launch-ltm output to match already-exists check")
	}

	if isAlreadyExistsOutput("ERROR: gcloud compute instances create failed\n", "The KCS instance already exists!") {
		t.Fatal("expected unrelated failure output not to match already-exists check")
	}
}

func postTaskRequestForTest(t *testing.T, reqBody TaskRequest, internal bool) (*httptest.ResponseRecorder, TaskRequest, error) {
	t.Helper()
	b, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/gce-xfstests", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	parsed, err := ParseTaskRequest(rec, req, internal)
	return rec, parsed, err
}

func TestParseTaskRequestGitRepoValidation(t *testing.T) {
	rec, _, err := postTaskRequestForTest(t, TaskRequest{
		Options: &UserOptions{
			GitRepo: "https://github.com/tytso/ext4.git",
		},
	}, false)
	if err != nil {
		t.Fatalf("expected valid git_repo to succeed, got err=%v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	for _, bad := range []string{
		"file:///etc/passwd",
		"file://localhost/tmp/repo.git",
		"--upload-pack=/bin/sh",
		"ext::sh -c evil",
		"https://github.com",
		"https://github.com/tytso/../ext4.git",
	} {
		rec, _, err := postTaskRequestForTest(t, TaskRequest{
			Options: &UserOptions{
				GitRepo: bad,
			},
		}, false)
		if err == nil {
			t.Fatalf("expected ParseTaskRequest to reject git_repo=%q", bad)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for git_repo=%q, got %d", bad, rec.Code)
		}
	}
}

func TestParseTaskRequestGitRefAndBranchValidation(t *testing.T) {
	rec, _, err := postTaskRequestForTest(t, TaskRequest{
		Options: &UserOptions{
			CommitID:   "HEAD~1",
			BranchName: "ext4/for-next",
			BadCommit:  "v6.12-rc1",
			GoodCommit: "v6.10|v6.11",
		},
	}, false)
	if err != nil || rec.Code != http.StatusOK {
		t.Fatalf("expected valid git refs to succeed, got code=%d err=%v", rec.Code, err)
	}

	badCases := []UserOptions{
		{CommitID: "--detach"},
		{CommitID: "HEAD..origin/main"},
		{BranchName: "--upload-pack=/bin/sh"},
		{BranchName: "HEAD~1"},
		{BranchName: "ext4/*"},
		{BadCommit: "--bad"},
		{GoodCommit: "v5.6||v5.7"},
		{GoodCommit: "v5.6|--good"},
		{GoodCommit: "|v5.6"},
	}
	for _, opts := range badCases {
		optsCopy := opts
		rec, _, err := postTaskRequestForTest(t, TaskRequest{Options: &optsCopy}, false)
		if err == nil || rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for %+v, got code=%d err=%v", opts, rec.Code, err)
		}
	}
}

func TestParseTaskRequestGCSFieldsValidation(t *testing.T) {
	rec, _, err := postTaskRequestForTest(t, TaskRequest{
		Options: &UserOptions{
			BucketSubdir: "results/ext4",
			GsKernel:     "gs://my-bucket/kernels/bzImage-test-onerun.deb",
			KConfig:      "gs://my-bucket/build_config",
		},
	}, false)
	if err != nil || rec.Code != http.StatusOK {
		t.Fatalf("expected valid GCS fields to succeed, got code=%d err=%v", rec.Code, err)
	}

	badCases := []UserOptions{
		{BucketSubdir: "../etc"},
		{BucketSubdir: "/results"},
		{BucketSubdir: "-results"},
		{BucketSubdir: "results/*"},
		{GsKernel: "/etc/passwd"},
		{GsKernel: "gs://my-bucket/../secret"},
		{KConfig: "/etc/shadow"},
		{KConfig: "gs://my-bucket/"},
	}
	for _, opts := range badCases {
		optsCopy := opts
		rec, _, err := postTaskRequestForTest(t, TaskRequest{Options: &optsCopy}, false)
		if err == nil || rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for %+v, got code=%d err=%v", opts, rec.Code, err)
		}
	}
}

func TestParseTaskRequestCmdLineValidation(t *testing.T) {
	validB64 := base64.StdEncoding.EncodeToString([]byte("gce-xfstests ltm -c ext4/4k smoke\n"))
	rec, _, err := postTaskRequestForTest(t, TaskRequest{CmdLine: validB64}, false)
	if err != nil || rec.Code != http.StatusOK {
		t.Fatalf("expected valid orig_cmdline to succeed, got code=%d err=%v", rec.Code, err)
	}

	badCmdLines := []string{
		"%%%not-base64%%%",
		base64.StdEncoding.EncodeToString([]byte("ltm -c ../../etc/passwd")),
		base64.StdEncoding.EncodeToString([]byte("ltm -c")),
		base64.StdEncoding.EncodeToString([]byte("ltm\x00smoke")),
		base64.StdEncoding.EncodeToString([]byte("ltm smoke\n--evil")),
	}
	for _, b64 := range badCmdLines {
		rec, _, err := postTaskRequestForTest(t, TaskRequest{CmdLine: b64}, false)
		if err == nil || rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for cmdline %q, got code=%d err=%v", b64, rec.Code, err)
		}
	}
}

func TestParseTaskRequestEmailArchTimeoutAndBuildOptsValidation(t *testing.T) {
	rec, _, err := postTaskRequestForTest(t, TaskRequest{
		Options: &UserOptions{
			ReportEmail:     "tytso@mit.edu",
			ReportFailEmail: "tytso@mit.edu, alerts@google.com",
			JunitEmail:      "junit@google.com",
			Arch:            "arm64",
			MonitorTimeout:  "30m",
			KConfigOpts:     "--kasan,--lockdep",
			KbuildOpts:      "--oldconfig -j 8",
		},
	}, false)
	if err != nil || rec.Code != http.StatusOK {
		t.Fatalf("expected valid options to succeed, got code=%d err=%v", rec.Code, err)
	}

	badCases := []UserOptions{
		{ReportEmail: "-f /etc/shadow"},
		{ReportFailEmail: "--config=/etc/passwd"},
		{JunitEmail: "not-an-email"},
		{Arch: "mips64"},
		{MonitorTimeout: "0s"},
		{MonitorTimeout: "-10m"},
		{MonitorTimeout: "bogus"},
		{KConfigOpts: "--kasan;id"},
		{KbuildOpts: "-j8 *"},
	}
	for _, opts := range badCases {
		optsCopy := opts
		rec, _, err := postTaskRequestForTest(t, TaskRequest{Options: &optsCopy}, false)
		if err == nil || rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for %+v, got code=%d err=%v", opts, rec.Code, err)
		}
	}
}

func generateTestCertFiles(t *testing.T, notBefore, notAfter time.Time, dnsSANs []string) (string, tls.Certificate) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "*.test-project.gce-xfstests"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsSANs,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	certFile := filepath.Join(t.TempDir(), "cert.pem")
	if err := os.WriteFile(certFile, certPEM, 0644); err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return certFile, pair
}

func runTLSHandshakeOverPipe(clientCfg *tls.Config, serverPair tls.Certificate) error {
	cConn, sConn := net.Pipe()
	defer cConn.Close()
	defer sConn.Close()

	srvTLS := tls.Server(sConn, &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverPair},
	})
	srvDone := make(chan struct{})
	go func() {
		defer close(srvDone)
		_ = srvTLS.Handshake()
		_ = srvTLS.Close()
	}()

	cliTLS := tls.Client(cConn, clientCfg)
	err := cliTLS.Handshake()
	_ = cliTLS.Close()
	<-srvDone
	return err
}

func TestStrictInternalTLSVerificationAndPinning(t *testing.T) {
	now := time.Now()
	validCertFile, validPair := generateTestCertFiles(t,
		now.Add(-time.Hour), now.Add(24*time.Hour),
		[]string{"*.test-project.gce-xfstests"},
	)
	_, roguePair := generateTestCertFiles(t,
		now.Add(-time.Hour), now.Add(24*time.Hour),
		[]string{"*.test-project.gce-xfstests"},
	)
	expiredCertFile, _ := generateTestCertFiles(t,
		now.Add(-48*time.Hour), now.Add(-time.Hour),
		[]string{"*.test-project.gce-xfstests"},
	)
	notYetValidCertFile, _ := generateTestCertFiles(t,
		now.Add(time.Hour), now.Add(24*time.Hour),
		[]string{"*.test-project.gce-xfstests"},
	)
	cnOnlyCertFile, cnOnlyPair := generateTestCertFiles(t,
		now.Add(-time.Hour), now.Add(24*time.Hour),
		nil,
	)

	// 1. Valid SAN cert matching pinned certFile succeeds.
	tlsCfg, err := buildInternalTLSConfig(validCertFile, "a.test-project.gce-xfstests")
	if err != nil {
		t.Fatalf("buildInternalTLSConfig(valid) failed: %v", err)
	}
	if err := runTLSHandshakeOverPipe(tlsCfg, validPair); err != nil {
		t.Fatalf("expected valid pinned TLS handshake to succeed, got: %v", err)
	}

	// 2. Rogue cert with identical SAN is rejected by exact DER pinning.
	if err := runTLSHandshakeOverPipe(tlsCfg, roguePair); err == nil {
		t.Fatal("expected rogue certificate to be rejected")
	}

	// 3. Expired and not-yet-valid certs are rejected strictly.
	if _, err := buildInternalTLSConfig(expiredCertFile, "a.test-project.gce-xfstests"); err == nil {
		t.Fatal("expected expired certificate to be rejected")
	}
	if _, err := buildInternalTLSConfig(notYetValidCertFile, "a.test-project.gce-xfstests"); err == nil {
		t.Fatal("expected not-yet-valid certificate to be rejected")
	}

	// 4. CN-only cert (missing SAN) is rejected by standard Go X.509 verification.
	cnCfg, err := buildInternalTLSConfig(cnOnlyCertFile, "a.test-project.gce-xfstests")
	if err != nil {
		t.Fatalf("buildInternalTLSConfig(cnOnly) failed: %v", err)
	}
	if err := runTLSHandshakeOverPipe(cnCfg, cnOnlyPair); err == nil {
		t.Fatal("expected CN-only certificate without SAN to be rejected")
	}
}

func TestSessionKeyPermissionsCookieFlagsBodyLimitAndStatusCodes(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, ".sessions_secret_key")

	// Creating a new session key writes mode 0600.
	k1, err := loadOrCreateSessionKey(keyPath)
	if err != nil || len(k1) != 32 {
		t.Fatalf("loadOrCreateSessionKey create failed: len=%d err=%v", len(k1), err)
	}
	st, err := os.Stat(keyPath)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 permissions on created session key, got %v (err=%v)", st.Mode().Perm(), err)
	}

	// Pre-existing 0644 key is tightened to 0600 on load.
	if err := os.Chmod(keyPath, 0644); err != nil {
		t.Fatal(err)
	}
	k2, err := loadOrCreateSessionKey(keyPath)
	if err != nil || !bytes.Equal(k1, k2) {
		t.Fatalf("loadOrCreateSessionKey reload mismatch: err=%v", err)
	}
	st, err = os.Stat(keyPath)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 permissions after reload, got %v", st.Mode().Perm())
	}

	inst, err := New("127.0.0.1:0", "test-http")
	if err != nil {
		t.Fatal(err)
	}
	defer inst.Close()

	inst.Handler().Handle("/panic", inst.FailureHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("simulated boom")
	}))).Methods("POST")
	inst.Handler().Handle("/protected", inst.LoginHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = SendResponse(w, r, SimpleResponse{Status: true, Msg: "authed"})
	}))).Methods("POST")

	// Wrong login password -> 403 JSON
	badLoginReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"password":"wrong"}`))
	badLoginRec := httptest.NewRecorder()
	inst.Handler().ServeHTTP(badLoginRec, badLoginReq)
	if badLoginRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on wrong login password, got %d", badLoginRec.Code)
	}

	// Valid login -> Set-Cookie has Secure, HttpOnly, SameSite=Strict and authenticates LoginHandler
	okLoginReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"password":"`+password+`"}`))
	okLoginRec := httptest.NewRecorder()
	inst.Handler().ServeHTTP(okLoginRec, okLoginReq)
	if okLoginRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on valid login, got %d", okLoginRec.Code)
	}
	cookies := okLoginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected Set-Cookie header on login")
	}
	sessionCookie := cookies[0]
	if !sessionCookie.HttpOnly || !sessionCookie.Secure || sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("expected HttpOnly+Secure+SameSiteStrict cookie, got HttpOnly=%v Secure=%v SameSite=%v",
			sessionCookie.HttpOnly, sessionCookie.Secure, sessionCookie.SameSite)
	}

	authedReq := httptest.NewRequest(http.MethodPost, "/protected", strings.NewReader(`{}`))
	authedReq.AddCookie(sessionCookie)
	authedRec := httptest.NewRecorder()
	inst.Handler().ServeHTTP(authedRec, authedReq)
	if authedRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on authenticated request, got %d", authedRec.Code)
	}

	// Malformed JSON -> 400 Bad Request JSON
	malformedReq := httptest.NewRequest(http.MethodPost, "/gce-xfstests", strings.NewReader(`{not-json`))
	malformedRec := httptest.NewRecorder()
	if _, err := ParseTaskRequest(malformedRec, malformedReq, false); err == nil || malformedRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on malformed JSON, got code=%d err=%v", malformedRec.Code, err)
	}

	// Oversized (>4 MiB) body -> 413 Request Entity Too Large
	hugeJSON := `{"orig_cmdline":"` + strings.Repeat("A", maxRequestBodyBytes+1024) + `"}`
	hugeReq := httptest.NewRequest(http.MethodPost, "/gce-xfstests", strings.NewReader(hugeJSON))
	hugeRec := httptest.NewRecorder()
	if _, err := ParseTaskRequest(hugeRec, hugeReq, false); err == nil || hugeRec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 on oversized request body, got code=%d err=%v", hugeRec.Code, err)
	}

	// FailureHandler panic -> 500 Internal Server Error JSON
	panicReq := httptest.NewRequest(http.MethodPost, "/panic", strings.NewReader(`{}`))
	panicRec := httptest.NewRecorder()
	inst.Handler().ServeHTTP(panicRec, panicReq)
	if panicRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 from FailureHandler, got %d", panicRec.Code)
	}
	var panicResp SimpleResponse
	if err := json.Unmarshal(panicRec.Body.Bytes(), &panicResp); err != nil || panicResp.Status || panicResp.Msg != "simulated boom" {
		t.Fatalf("unexpected FailureHandler JSON body: %q (err=%v)", panicRec.Body.String(), err)
	}
}
