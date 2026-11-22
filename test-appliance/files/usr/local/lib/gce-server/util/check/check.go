/*
Package check executes external commands, performs File I/Os and issues
OS related commands.

It also checks for errors and writes messages into logger.
*/
package check

import (
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

// RootDir points to the root of go server source code.
// The compiled go executables are located in GOPATH/bin
const RootDir = "/usr/local/lib/gce-server"

// cmdCap caps the number of gce-xfstests commands that can run at the same time.
// cmdLimit limits the frequency at which these commands can be called.
// They reduce the risk of exhausting memory when launching test VMs.
const (
	cmdCap   = 12
	cmdLimit = 1
)

var (
	capper  = make(chan struct{}, cmdCap)
	limiter = rate.NewLimiter(rate.Every(cmdLimit*time.Second), 1)
)

// EmptyEnv provides a placeholder for default exec environment.
var EmptyEnv = map[string]string{}

// Run executes an external command and checks the return status.
// Returns true on success and false otherwise.
func Run(cmd *exec.Cmd, workDir string, env map[string]string, stdout io.Writer, stderr io.Writer) error {
	cmd.Dir = workDir
	cmd.Env = parseEnv(env)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// Output executes an external command, checks the return status, and
// returns the command stdout.
func Output(cmd *exec.Cmd, workDir string, env map[string]string, stderr io.Writer) (string, error) {
	cmd.Dir = workDir
	cmd.Env = parseEnv(env)
	cmd.Stderr = stderr
	out, err := cmd.Output()
	return string(out), err
}

// LimitedRun works like Run but caps the number of running commands.
// At most cmdCap commands passed by LimitedRun can run at the same time.
func LimitedRun(cmd *exec.Cmd, workDir string, env map[string]string, stdout io.Writer, stderr io.Writer) error {
	capper <- struct{}{}
	defer func() { <-capper }()
	err := limiter.Wait(context.Background())
	if err != nil {
		return err
	}
	return Run(cmd, workDir, env, stdout, stderr)
}

// LimitedOutput works like Output but caps the number of running commands.
func LimitedOutput(cmd *exec.Cmd, workDir string, env map[string]string, stderr io.Writer) (string, error) {
	capper <- struct{}{}
	defer func() { <-capper }()
	err := limiter.Wait(context.Background())
	if err != nil {
		return "", err
	}
	return Output(cmd, workDir, env, stderr)
}

// CombinedOutput executes an external command, checks the return status, and
// returns the combined stdout and stderr.
func CombinedOutput(cmd *exec.Cmd, workDir string, env map[string]string) (string, error) {
	cmd.Dir = workDir
	cmd.Env = parseEnv(env)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// parseEnv adds user specified environment to os.Environ.
func parseEnv(env map[string]string) []string {
	newEnv := os.Environ()
	for key, value := range env {
		newEnv = append(newEnv, key+"="+value)
	}
	return newEnv
}

// CreateDir creates a directory recursively with default permissions.
func CreateDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// FileExists returns true if a file exists.
func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if err == nil && !info.IsDir() {
		return true
	}
	return false
}

// DirExists returns true is a directory exists.
func DirExists(filename string) bool {
	info, err := os.Stat(filename)
	if err == nil && info.IsDir() {
		return true
	}
	return false
}

// ReadLines read a whole file into a slice of strings split by newlines,
// and removes '\n' and empty lines.
func ReadLines(filename string) ([]string, error) {
	lines := []string{}

	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return lines, err
	}
	lines = strings.Split(string(content), "\n")
	nonEmptyLines := lines[:0]
	for i, line := range lines {
		if line != "" {
			nonEmptyLines = append(nonEmptyLines, lines[i:i+1]...)
		}
	}
	return nonEmptyLines, nil
}

// CopyFile copies the content of file src to file dst.
// Overwrites dst if it already exists.
func CopyFile(dst string, src string) error {
	from, err := os.Open(src)
	if err != nil {
		return err
	}
	defer from.Close()

	if FileExists(dst) {
		err = os.Remove(dst)
		if err != nil {
			return err
		}
	}

	to, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer to.Close()

	_, err = io.Copy(to, from)
	if err != nil {
		return err
	}
	return nil
}

// Panic checks an error and logs a panic entry with given msg
// if the error is not nil.
func Panic(err error, log *logrus.Entry, msg string) {
	if msg == "" {
		msg = "Something bad happended"
	}
	if err != nil {
		log.WithError(err).Panic(msg)
	}
}

// NoError checks an error and logs a error entry with given msg
// if the error is not nil. Returns true otherwise.
func NoError(err error, log *logrus.Entry, msg string) bool {
	if msg == "" {
		msg = "Something bad happended"
	}
	if err != nil {
		log.WithError(err).Error(msg)
		return false
	}
	return true
}

// ContainsStr searches an array of strings for a target string,
// returning true if the target is found and false otherwise.
func ContainsStr(strings []string, target string) bool {
	for _, s := range strings {
		if s == target {
			return true
		}
	}
	return false
}

// ValidateTestID validates that testID is a non-empty single path component
// safe for use in filenames and directory paths. It rejects empty strings,
// leading '-' characters, "." and ".." traversal segments, path separators
// ('/' and '\\'), NUL bytes, and ASCII whitespace or control characters.
func ValidateTestID(testID string) error {
	if testID == "" {
		return fmt.Errorf("testID must not be empty")
	}
	if strings.HasPrefix(testID, "-") {
		return fmt.Errorf("testID must not start with '-'")
	}
	if testID == "." || testID == ".." || strings.Contains(testID, "..") {
		return fmt.Errorf("testID must not contain '..' or be '.'")
	}
	if strings.ContainsAny(testID, "/\\\x00") {
		return fmt.Errorf("testID must not contain path separators or NUL bytes")
	}
	for i := 0; i < len(testID); i++ {
		if testID[i] <= 0x20 || testID[i] == 0x7f {
			return fmt.Errorf("testID must not contain whitespace or control characters")
		}
	}
	return nil
}

// ValidPassword compares provided against expected in constant time,
// rejecting empty expected passwords.
func ValidPassword(provided, expected string) bool {
	return expected != "" &&
		subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// ValidateGitRepoURL validates a remote git repository URL accepted by LTM/KCS
// HTTP endpoints. Only http, https, and git URL schemes with a valid hostname
// and clean repository path are permitted.
func ValidateGitRepoURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("git repo URL must not be empty")
	}
	if strings.HasPrefix(rawURL, "-") {
		return fmt.Errorf("git repo URL must not start with '-'")
	}
	if strings.ContainsAny(rawURL, "\\\x00") {
		return fmt.Errorf("git repo URL must not contain backslashes or NUL bytes")
	}
	for i := 0; i < len(rawURL); i++ {
		if rawURL[i] <= 0x20 || rawURL[i] == 0x7f {
			return fmt.Errorf("git repo URL must not contain whitespace or control characters")
		}
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid git repo URL: %w", err)
	}
	switch u.Scheme {
	case "http", "https", "git":
	default:
		return fmt.Errorf("unsupported git repo URL scheme %q (must be http, https, or git)", u.Scheme)
	}
	if u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("git repo URL must not contain userinfo, query, or fragment")
	}
	host := u.Hostname()
	if host == "" || strings.HasPrefix(host, "-") || strings.Contains(host, "..") {
		return fmt.Errorf("git repo URL must contain a valid hostname")
	}
	if port := u.Port(); port != "" {
		for i := 0; i < len(port); i++ {
			if port[i] < '0' || port[i] > '9' {
				return fmt.Errorf("git repo URL contains invalid port")
			}
		}
	}
	trimmedPath := strings.Trim(u.Path, "/")
	if trimmedPath == "" {
		return fmt.Errorf("git repo URL must include a repository path")
	}
	for _, seg := range strings.Split(trimmedPath, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.Contains(seg, "..") {
			return fmt.Errorf("git repo URL contains invalid path segment %q", seg)
		}
	}
	return nil
}

// ValidateGitRef validates a git revision or commit-ish (such as a SHA, tag,
// branch, or relative ref like HEAD~1). It rejects empty strings, leading '-',
// '..' range/traversal sequences, '@{' reflog expressions, backslashes, colons,
// glob characters, leading/trailing/duplicate '/', '.'/'..' segments, trailing
// '.lock', NUL bytes, and ASCII whitespace or control characters.
func ValidateGitRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("git revision must not be empty")
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("git revision must not start with '-'")
	}
	if strings.Contains(ref, "..") || strings.Contains(ref, "@{") {
		return fmt.Errorf("git revision must not contain '..' or '@{'")
	}
	if strings.ContainsAny(ref, "\\:\x00?*[]") {
		return fmt.Errorf("git revision contains invalid characters")
	}
	if strings.HasPrefix(ref, "/") || strings.HasSuffix(ref, "/") || strings.HasSuffix(ref, ".") {
		return fmt.Errorf("git revision must not start or end with '/' or end with '.'")
	}
	for i := 0; i < len(ref); i++ {
		if ref[i] <= 0x20 || ref[i] == 0x7f {
			return fmt.Errorf("git revision must not contain whitespace or control characters")
		}
	}
	for _, seg := range strings.Split(ref, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.HasPrefix(seg, ".") || strings.HasSuffix(seg, ".lock") {
			return fmt.Errorf("git revision contains invalid component %q", seg)
		}
	}
	return nil
}

// ValidateGitBranch validates a git branch name passed to git ls-remote.
// In addition to ValidateGitRef rules, it rejects '~', '^', and bare '@'.
func ValidateGitBranch(branch string) error {
	if branch == "@" {
		return fmt.Errorf("git branch must not be '@'")
	}
	if err := ValidateGitRef(branch); err != nil {
		return fmt.Errorf("invalid git branch: %w", err)
	}
	if strings.ContainsAny(branch, "~^") {
		return fmt.Errorf("git branch must not contain '~' or '^'")
	}
	return nil
}

// ValidateBucketSubdir validates a relative GCS subdirectory prefix (such as
// "results" or "results/ext4"). It rejects empty strings, leading/trailing or
// duplicate '/', leading '-' segments, '.' or '..' traversal segments,
// backslashes, wildcards, NUL bytes, and ASCII whitespace or control chars.
func ValidateBucketSubdir(subdir string) error {
	if subdir == "" {
		return fmt.Errorf("bucket_subdir must not be empty")
	}
	if strings.HasPrefix(subdir, "/") || strings.HasSuffix(subdir, "/") {
		return fmt.Errorf("bucket_subdir must be a relative path without leading or trailing '/'")
	}
	if strings.ContainsAny(subdir, "\\\x00*?[]") {
		return fmt.Errorf("bucket_subdir contains invalid characters")
	}
	for i := 0; i < len(subdir); i++ {
		if subdir[i] <= 0x20 || subdir[i] == 0x7f {
			return fmt.Errorf("bucket_subdir must not contain whitespace or control characters")
		}
	}
	for _, seg := range strings.Split(subdir, "/") {
		if seg == "" {
			return fmt.Errorf("bucket_subdir must not contain empty path segments")
		}
		if err := ValidateTestID(seg); err != nil {
			return fmt.Errorf("invalid bucket_subdir segment %q: %w", seg, err)
		}
	}
	return nil
}

// ValidateGSURI validates a gs://<bucket>/<object> Cloud Storage object URI.
// It rejects non-gs:// paths, invalid bucket names, empty or trailing-slash
// object names, '.'/'..' traversal, duplicate slashes, backslashes, wildcards,
// NUL bytes, and ASCII whitespace or control characters.
func ValidateGSURI(uri string) error {
	if !strings.HasPrefix(uri, "gs://") {
		return fmt.Errorf("GCS URI must start with gs://")
	}
	if strings.ContainsAny(uri, "\\\x00*?[]") {
		return fmt.Errorf("GCS URI contains invalid characters")
	}
	for i := 0; i < len(uri); i++ {
		if uri[i] <= 0x20 || uri[i] == 0x7f {
			return fmt.Errorf("GCS URI must not contain whitespace or control characters")
		}
	}
	rest := strings.TrimPrefix(uri, "gs://")
	bucket, object, ok := strings.Cut(rest, "/")
	if !ok || bucket == "" || object == "" {
		return fmt.Errorf("GCS URI must have form gs://<bucket>/<object>")
	}
	if len(bucket) < 3 || len(bucket) > 222 || strings.Contains(bucket, "..") {
		return fmt.Errorf("GCS URI contains invalid bucket name %q", bucket)
	}
	isAlnum := func(b byte) bool {
		return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
	}
	if !isAlnum(bucket[0]) || !isAlnum(bucket[len(bucket)-1]) {
		return fmt.Errorf("GCS bucket name %q must start and end with a lowercase letter or digit", bucket)
	}
	for i := 0; i < len(bucket); i++ {
		b := bucket[i]
		if !isAlnum(b) && b != '-' && b != '_' && b != '.' {
			return fmt.Errorf("GCS bucket name %q contains invalid character %q", bucket, b)
		}
	}
	if strings.HasPrefix(object, "/") || strings.HasSuffix(object, "/") {
		return fmt.Errorf("GCS object path must not start or end with '/'")
	}
	for _, seg := range strings.Split(object, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.Contains(seg, "..") {
			return fmt.Errorf("GCS object path contains invalid segment %q", seg)
		}
	}
	return nil
}
