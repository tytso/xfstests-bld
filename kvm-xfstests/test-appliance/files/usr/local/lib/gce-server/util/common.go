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
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// RootDir points to the root of go server source code
// The compiled go executables are located in GOPATH/bin
const RootDir = "/usr/local/lib/gce-server"

// EmptyEnv provides a placeholder for default exec environment.
var EmptyEnv = map[string]string{}
var idMutex sync.Mutex

// CheckRun executes an external command and checks the return status.
// Returns true on success and false otherwise.
func CheckRun(cmd *exec.Cmd, workDir string, env map[string]string, stdout io.Writer, stderr io.Writer) error {
	cmd.Dir = workDir
	cmd.Env = parseEnv(env)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	return err
}

// CheckOutput executes an external command, checks the return status, and
// returns the command stdout.
func CheckOutput(cmd *exec.Cmd, workDir string, env map[string]string, stderr io.Writer) (string, error) {
	cmd.Dir = workDir
	cmd.Env = parseEnv(env)
	cmd.Stderr = stderr
	out, err := cmd.Output()
	return string(out), err
}

// CheckCombinedOutput executes an external command, checks the return status, and
// returns the combined stdout and stderr.
func CheckCombinedOutput(cmd *exec.Cmd, workDir string, env map[string]string) (string, error) {
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

// CreateDir creates a directory with default permissions.
func CreateDir(path string) error {
	err := os.MkdirAll(path, 0755)
	return err
}

// RemoveDir removes a directory and all contents in it.
// Do nothing if the target path doesn't exist.
func RemoveDir(path string) error {
	err := os.RemoveAll(path)
	return err
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

// CopyFile copies the content of file src to file dst
// Overwrites dst if it already exists
func CopyFile(dst string, src string) error {
	from, err := os.Open(src)
	if err != nil {
		return err
	}
	defer from.Close()

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
