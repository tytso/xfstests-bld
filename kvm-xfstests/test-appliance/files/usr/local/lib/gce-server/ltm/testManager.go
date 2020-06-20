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
	sharder        shardSchedular
	gsBucket       string
	bucketSubdir   string
	gsKernel       string
	reportReceiver string
}

func newTestManager(c LTMRequest) *testManager {
	tester := new(testManager)
	tester.id = getTimeStamp()
	tester.origCmd = strings.TrimSpace(c.CmdLine)

	logDir := TestLogPath + "/" + tester.id
	tester.logFile = logDir + "/run.log"
	tester.aggPath = logDir + "/results-ltm-" + tester.id
	tester.aggFile = logDir + "results.ltm-" + tester.id
	tester.kernelVersion = "unknown_kernel_version"

	util.CreateDir(logDir)
	config := util.GetConfig()
	tester.gsBucket = config.Get("GS_BUCKET")
	tester.bucketSubdir = config.Get("BUCKET_SUBDIR")

	regionShard := !c.Options.NoRegionShard
	tester.bucketSubdir = c.Options.BucketSubdir
	tester.gsKernel = c.Options.GsKernel
	tester.reportReceiver = c.Options.ReportEmail

	tester.sharder = newShardSchedular(tester.origCmd, tester.id, logDir,
		tester.gsKernel, regionShard, 0)

	return tester
}

func (tester *testManager) run() LTMRespond {
	return LTMRespond{true}
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
