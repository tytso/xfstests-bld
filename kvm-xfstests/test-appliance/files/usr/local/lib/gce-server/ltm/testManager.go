package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"example.com/gce-server/util"
)

var idMutex sync.Mutex

type testManager struct {
	id             string
	origCmd        string
	logFile        string
	aggPath        string
	aggFile        string
	kernelVersion  string
	sharder        *shardSchedular
	gsBucket       string
	bucketSubdir   string
	gsKernel       string
	reportReceiver string
}

func NewTestManager(c LTMRequest) *testManager {
	tester := new(testManager)
	tester.id = getTimeStamp()
	tester.origCmd = strings.TrimSpace(c.CmdLine)

	logDir := TestLogDir + tester.id + "/"
	tester.logFile = logDir + "run.log"
	tester.aggPath = fmt.Sprintf("%sresults-%s-%s/", logDir, LTMUserName, tester.id)
	tester.aggFile = fmt.Sprintf("%sresults.%s-%s", logDir, LTMUserName, tester.id)
	tester.kernelVersion = "unknown_kernel_version"

	util.CreateDir(logDir)
	config := util.GetConfig()
	tester.gsBucket = config.Get("GS_BUCKET")
	tester.bucketSubdir = config.Get("BUCKET_SUBDIR")

	regionShard := !c.Options.NoRegionShard
	if c.Options.BucketSubdir != "" {
		tester.bucketSubdir = c.Options.BucketSubdir
	}
	if c.Options.GsKernel != "" {
		tester.gsKernel = c.Options.GsKernel
	}
	if c.Options.ReportEmail != "" {
		tester.reportReceiver = c.Options.ReportEmail
	}

	tester.sharder = NewShardSchedular(tester.origCmd, tester.id, logDir,
		tester.bucketSubdir, tester.gsKernel, regionShard, 0)

	return tester
}

func (tester *testManager) Run() SharderInfo {
	go tester.sharder.Run()

	return tester.sharder.Info()
}

func getTimeStamp() string {
	idMutex.Lock()
	defer idMutex.Unlock()
	// inefficient way to avoid duplicate timestamp
	time.Sleep(2 * time.Second)
	t := time.Now()
	return fmt.Sprintf("%.4d%.2d%.2d%.2d%.2d%.2d",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
}
