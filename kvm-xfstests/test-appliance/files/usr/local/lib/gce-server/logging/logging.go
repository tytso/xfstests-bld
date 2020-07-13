package logging

import (
	"io"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// log file paths for go server components
const (
	ServerLogPath = "/var/log/go/go.log"
	LTMLogDir     = "/var/log/go/ltm_logs/"
	KCSLogDir     = "/var/log/go/kcs_logs/"
)

// DEBUG declares debugging runs, where log prints to stdout and
// mock modules are used.
const DEBUG = true

// InitLogger initializes a logrus logger and writes to logfile
// writes to stdout if cannot open logfile
func InitLogger(logfile string) *logrus.Entry {
	log := logrus.New()
	file, err := os.OpenFile(logfile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		log.Out = file
	} else {
		log.Out = os.Stdout
		log.WithError(err).Warn("Failed to open log file, use os.Stdout instead")
	}
	log.SetLevel(logrus.DebugLevel)
	log.SetFormatter(&logrus.TextFormatter{})
	log.SetReportCaller(true)

	if DEBUG {
		log.Out = os.Stdout
		log.SetReportCaller(false)
	}

	return logrus.NewEntry(log)
}

// CloseLog closes the log file handler. It does nothing if log writes
// to os.Stdout or os.Stderr.
// TODO: find a more elegant way for the checking
func CloseLog(log *logrus.Entry) {
	if file, ok := log.Logger.Out.(*os.File); ok {
		if !strings.HasPrefix(file.Name(), "/dev") {
			file.Sync()
			file.Close()
		}
	} else if handler, ok := log.Logger.Out.(io.Closer); ok {
		handler.Close()
	}
}

// CheckPanic checks an error and log a panic entry with given msg
func CheckPanic(err error, log *logrus.Entry, msg string) {
	if msg == "" {
		msg = "Something bad happended"
	}
	if err != nil {
		log.WithError(err).Panic(msg)
	}
}

// CheckNoError checks an error and log a error entry with given msg
// return true if error is nil
func CheckNoError(err error, log *logrus.Entry, msg string) bool {
	if msg == "" {
		msg = "Something bad happended"
	}
	if err != nil {
		log.WithError(err).Error(msg)
		return false
	}
	return true
}

// Sync flushes the log to disk file
func Sync(log *logrus.Entry) {
	if file, ok := log.Logger.Out.(*os.File); ok {
		if !strings.HasPrefix(file.Name(), "/dev") {
			file.Sync()
		}
	}
}

// GetFile returns the log file descripter if it exists
// return nil otherwise
func GetFile(log *logrus.Entry) *os.File {
	if file, ok := log.Logger.Out.(*os.File); ok {
		return file
	}
	return nil
}
