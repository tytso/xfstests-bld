/*
Package check executes external commands, performs File I/Os and issues
OS related commands.

It also checks for errors and writes messages into logger.
*/
package check

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

// RootDir points to the root of go server source code
// The compiled go executables are located in GOPATH/bin
const RootDir = "/usr/local/lib/gce-server"

// cmdCap caps the number of gce-xfstests commands that can run at the same time.
// cmdLimit limits the frequency at which these commands can be called.
// They reduce the risk of exhausting memory when launching test VMs.
const (
	cmdCap   = 15
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
// At most cmdLimit commands passed by LimitedRun can run at the same time.
func LimitedRun(cmd *exec.Cmd, workDir string, env map[string]string, stdout io.Writer, stderr io.Writer) error {
	capper <- struct{}{}
	err := limiter.Wait(context.Background())
	if err != nil {
		return err
	}
	err = Run(cmd, workDir, env, stdout, stderr)
	<-capper
	return err
}

// LimitedOutput works like Output but caps the number of running commands.
func LimitedOutput(cmd *exec.Cmd, workDir string, env map[string]string, stderr io.Writer) (string, error) {
	capper <- struct{}{}
	err := limiter.Wait(context.Background())
	if err != nil {
		return "", err
	}
	output, err := Output(cmd, workDir, env, stderr)
	<-capper
	return output, err
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
