package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"example.com/gce-server/util"
	"google.golang.org/api/compute/v1"
)

type shardWorker struct {
	sharder            *shardSchedular
	shardID            string
	name               string
	zone               string
	config             string
	args               []string
	logPath            string
	cmdLogPath         string
	serialOutputPath   string
	resultsName        string
	tmpResultsDir      string
	unpackedResultsDir string
	vmTimeout          bool
}

type ShardInfo struct {
	Index   int    `json:"index"`
	ShardID string `json:"shard_id"`
	Config  string `json:"cfg"`
	Zone    string `json:"zone"`
}

func NewShardWorker(sharder *shardSchedular, shardID string, config string, zone string, validArgs []string) *shardWorker {
	shard := shardWorker{
		sharder: sharder,
		shardID: shardID,
		zone:    zone,
		config:  config,
	}
	shard.name = fmt.Sprintf("xfstests-ltm-%s-%s", sharder.testID, shardID)
	shard.args = []string{
		"--instance-name", shard.name,
		"--gce-zone", zone,
		"--gs-bucket", sharder.gsBucket,
		"--image-project", sharder.projID,
		"--no-email",
		"-c", config,
	}
	if sharder.gsKernel != "" {
		shard.args = append(shard.args, "--kernel", sharder.gsKernel)
	}
	if sharder.bucketSubdir != "" {
		shard.args = append(shard.args, "--bucket-subdir", sharder.bucketSubdir)
	}
	shard.args = append(shard.args, validArgs...)
	shard.setupLogging()

	return &shard
}

func (shard *shardWorker) setupLogging() {
	shard.logPath = shard.sharder.logDir + shard.shardID
	shard.cmdLogPath = shard.logPath + ".cmdlog"
	shard.serialOutputPath = shard.logPath + ".serial"
	shard.resultsName = fmt.Sprintf("%s-%s-%s", LTMUserName, shard.sharder.testID, shard.shardID)
	shard.tmpResultsDir = fmt.Sprintf("/tmp/results-%s-%s-%s", LTMUserName, shard.sharder.testID, shard.shardID)
	shard.unpackedResultsDir = fmt.Sprintf("%sresults-%s-%s-%s", shard.sharder.logDir, LTMUserName, shard.sharder.testID, shard.shardID)
}

func (shard *shardWorker) Run(wg *sync.WaitGroup) {
	defer wg.Done()
	file, err := os.Create(shard.cmdLogPath)
	util.Check(err)
	defer util.Close(file)
	cmd := exec.Command("gce-xfstests", shard.args...)
	log.Printf("%+v", cmd)
	status := util.CheckRun(cmd, util.Rootdir, util.EmptyEnv, file, file)
	if !status {
		log.Printf("Shard %s failed to start, args was:\n%s\n", shard.shardID, shard.args)
	} else {
		returnVal := shard.monitor()
		shard.cleanup(returnVal)
	}
	log.Printf("Existing monitor process for shard %s", shard.shardID)
}

func (shard *shardWorker) monitor() bool {
	var (
		waitTime       int
		timePrevStatus int
		prevStart      int64
		prevStatus     string
	)

	for true {
		time.Sleep(5 * time.Second)
		waitTime += 5

		instanceInfo, err := shard.sharder.gce.GetInstanceInfo(shard.sharder.projID, shard.zone, shard.name)

		if err != nil {
			if util.IsNotFound(err) {
				log.Printf("Test VM no longer exists\n")
				break
			}
			log.Fatal(err)
		}

		prevStart = shard.updateSerialData(prevStart)

		log.Printf("waitTime: %d status: %s\n", waitTime, prevStatus)

		if instanceInfo.Status != prevStatus {
			timePrevStatus = waitTime
			prevStatus = instanceInfo.Status
		} else if waitTime > timePrevStatus+10 {
			log.Printf("Instance seems to have wedged, no status update for > 1 hour.\n")
			log.Printf("Wait time: %d stayed at status %s since time: %d", waitTime, prevStatus, timePrevStatus)
			if !shard.sharder.keepDeadVM {
				shard.shutdownOnTimeout(instanceInfo.Metadata)
			} else {
				return false
			}
		}
	}
	return true

}

func (shard *shardWorker) updateSerialData(prevStart int64) int64 {
	output := shard.sharder.gce.GetSerialPortOutput(
		shard.sharder.projID, shard.zone, shard.name, prevStart)

	file, err := os.OpenFile(shard.serialOutputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	util.Check(err)
	defer util.Close(file)

	if output.Start > prevStart {
		_, err := file.WriteString(fmt.Sprintf(
			"\n!=====Missing data from %d to %d=====!\n", prevStart, output.Start))
		util.Check(err)
	}
	_, err = file.WriteString(output.Contents)
	util.Check(err)

	return output.Next
}

func (shard *shardWorker) shutdownOnTimeout(metadata *compute.Metadata) {
	shard.vmTimeout = true
	log.Printf("metadata: %+v", metadata)

	for _, item := range metadata.Items {
		log.Printf("metadata item: %+v", item)
		if item.Key == "shutdown_reason" {
			return
		}
	}

	val := "ltm detected test timeout"
	metadata.Items = append(metadata.Items, &compute.MetadataItems{
		Key:   "shutdown_reason",
		Value: &val,
	})

	shard.sharder.gce.SetMetadata(shard.sharder.projID, shard.zone, shard.name, metadata)
	// allow some time for metadata to be set
	time.Sleep(1 * time.Second)
	shard.sharder.gce.DeleteInstance(shard.sharder.projID, shard.zone, shard.name)
}

func (shard *shardWorker) cleanup(success bool) {

}

func (shard *shardWorker) Info(index int) ShardInfo {
	return ShardInfo{
		Index:   index,
		ShardID: shard.shardID,
		Config:  shard.config,
		Zone:    shard.zone,
	}
}
