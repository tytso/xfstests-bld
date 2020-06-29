/*
Package util implements some utility functions for LTM and KCS server.

The files in this library include:
	common.go: utility functions to check errors, execute external commands, i/o and os operations.
	gce.go: Google Compute Engine and Google Cloud Storage utilities
	git.go: git related utilities
	parser.go: gce-xfstests configuration parser
	set.go: set utilities

*/
package util

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// configurable constants shared between LTM and KCS
const (
	RootDir       = "/usr/local/lib/gce-server"
	ServerLogPath = "/var/log/lgtm/lgtm.log"
	TestLogDir    = "/var/log/lgtm/ltm_logs/"
	SecretPath    = "/etc/lighttpd/server.pem"
	CertPath      = "/root/xfstests_bld/kvm-xfstests/.gce_xfstests_cert.pem"
)

// EmptyEnv provides a placeholder for default exec environment.
var EmptyEnv = map[string]string{}
var idMutex sync.Mutex

// Check whether an error is not nil
func Check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// CheckRun executes an external command and checks the return status.
// Returns true on success and false otherwise.
func CheckRun(cmd *exec.Cmd, workDir string, env map[string]string, stdout io.Writer, stderr io.Writer) bool {
	cmd.Dir = workDir
	cmd.Env = parseEnv(env)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if err != nil {
		log.Fatalf("%s failed with error: %s\n", cmd.String(), err)
		return false
	}
	return true
}

// CheckOutput executes an external command, checks the return status, and
// returns the command stdout.
func CheckOutput(cmd *exec.Cmd, workDir string, env map[string]string, stderr io.Writer) (string, bool) {
	cmd.Dir = workDir
	cmd.Env = parseEnv(env)
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil {
		log.Fatalf("%s failed with error: %s\n", cmd.String(), err)
		return "", false
	}
	return string(out), true
}

// parseEnv adds user specified environment to os.Environ.
func parseEnv(env map[string]string) []string {
	newEnv := os.Environ()
	for key, value := range env {
		newEnv = append(newEnv, key+"="+value)
	}
	return newEnv
}

// CreateDir creates a directory with default permissions.
func CreateDir(path string) {
	err := os.MkdirAll(path, 0755)
	Check(err)
}

// RemoveDir removes a directory and all contents in it.
// Do nothing if the target path doesn't exist.
func RemoveDir(path string) {
	err := os.RemoveAll(path)
	Check(err)
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

// MinInt returns the smaller int.
func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// MaxInt returns the larger int.
func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// MaxIntSlice returns the largest int in a slice.
func MaxIntSlice(slice []int) (int, error) {
	if len(slice) == 0 {
		return 0, errors.New("MaxIntSlice: empty slice")
	}
	max := slice[0]
	for _, i := range slice[1:] {
		max = MaxInt(max, i)
	}
	return max, nil
}

// MinIntSlice returns the smallest int in a slice.
func MinIntSlice(slice []int) (int, error) {
	if len(slice) == 0 {
		return 0, errors.New("MaxIntSlice: empty slice")
	}
	max := slice[0]
	for _, i := range slice[1:] {
		max = MinInt(max, i)
	}
	return max, nil
}

// ReadLines read a whole file into a slice of strings split by newlines.
// Removes '\n' and empty lines
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

// Close a file handler and checks error
func Close(file *os.File) {
	if err := file.Close(); err != nil {
		log.Fatal(err)
	}
}

// GetTimeStamp returns the current timestamp
// Guaranteed uniqueness across go routines.
func GetTimeStamp() string {
	idMutex.Lock()
	defer idMutex.Unlock()
	// TODO: avoid duplicate timestamp with more efficient ways
	time.Sleep(2 * time.Second)
	t := time.Now()
	return fmt.Sprintf("%.4d%.2d%.2d%.2d%.2d%.2d",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
}
