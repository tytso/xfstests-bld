/*
ShardWorker launches, monitors and collects results from a single gce-xfstests run.

A shard is created and configured by a sharder only. It make a call to the gce-xfstests
scripts on start, and then waits for the test to finish by checking the VM status every
60 seconds. After the VM is deleted, the shard calls the scripts again to fetch the test
result files from GS and unpacks them to a local directory.

*/
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"gce-server/util"

	"google.golang.org/api/compute/v1"
)

// ShardWorker manages a single test VM.
type ShardWorker struct {
	sharder            *ShardSchedular
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

// ShardInfo exports shard info to be sent back to user.
type ShardInfo struct {
	Index   int    `json:"index"`
	ShardID string `json:"shard_id"`
	Config  string `json:"cfg"`
	Zone    string `json:"zone"`
}

// NewShardWorker constructs a new shard, requested by the sharder
func NewShardWorker(sharder *ShardSchedular, shardID string, config string, zone string, validArgs []string) *ShardWorker {
	shard := ShardWorker{
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
		"--kernel", sharder.gsKernel,
		"--bucket-subdir", sharder.bucketSubdir,
		"--no-email",
		"-c", config,
	}
	shard.args = append(shard.args, validArgs...)
	shard.setupLogging()

	return &shard
}

func (shard *ShardWorker) setupLogging() {
	shard.logPath = shard.sharder.logDir + shard.shardID
	shard.cmdLogPath = shard.logPath + ".cmdlog"
	shard.serialOutputPath = shard.logPath + ".serial"
	shard.resultsName = fmt.Sprintf("%s-%s-%s", LTMUserName, shard.sharder.testID, shard.shardID)
	shard.tmpResultsDir = fmt.Sprintf("/tmp/results-%s-%s-%s", LTMUserName, shard.sharder.testID, shard.shardID)
	shard.unpackedResultsDir = fmt.Sprintf("%sresults-%s-%s-%s", shard.sharder.logDir, LTMUserName, shard.sharder.testID, shard.shardID)
}

// Run issues the gce-xfstests command to launch a test vm and monitor its running status
func (shard *ShardWorker) Run(wg *sync.WaitGroup) {
	defer wg.Done()

	file, err := os.Create(shard.cmdLogPath)
	util.Check(err)
	cmd := exec.Command("gce-xfstests", shard.args...)
	log.Printf("%+v", cmd)
	err = util.CheckRun(cmd, util.RootDir, util.EmptyEnv, file, file)
	util.Close(file)

	if err != nil {
		log.Printf("Shard failed to start with error: %s. cmd: %s", err, cmd.String())
	} else {
		returnVal := shard.monitor()
		shard.finish(returnVal)
	}
	log.Printf("Existing monitor process for shard %s", shard.shardID)
}

// monitor queries the GCE api every minute and logs the serial console output
// to a local file. If the vm no longer exists or the status hasn't changed for
// more than an hour, the monitor kills the test vm.
// Returns true if the test vm exits normally.
func (shard *ShardWorker) monitor() bool {
	var (
		waitTime       int
		timePrevStatus int
		prevStart      int64
		prevStatus     string
	)

	for true {
		time.Sleep(60 * time.Second)
		waitTime += 60

		instanceInfo, err := shard.sharder.gce.GetInstanceInfo(shard.sharder.projID, shard.zone, shard.name)

		if err != nil {
			if util.IsNotFound(err) {
				// If prevStatus is empty, it's likely the VM never launched
				if prevStatus == "" {
					log.Printf("Test VM failed to launch")
				} else {
					log.Printf("Test VM no longer exists")
				}
				break
			}
			log.Fatal(err)
		}

		prevStart = shard.updateSerialData(prevStart)

		if instanceInfo.Status != prevStatus {
			timePrevStatus = waitTime
			prevStatus = instanceInfo.Status
		} else if waitTime > timePrevStatus+3600 {
			log.Printf("Instance seems to have wedged, no status update for > 1 hour.\n")
			log.Printf("Wait time: %d stayed at status %s since time: %d", waitTime, prevStatus, timePrevStatus)
			if !shard.sharder.keepDeadVM {
				shard.shutdownOnTimeout(instanceInfo.Metadata)
			} else {
				//TODO: improve the return logic here
				return false
			}
		}

		log.Printf("wait time: %d status: %s\n", waitTime, prevStatus)
	}
	return true

}

// updateSerialData writes the serial port output from the test vm to local log file.
func (shard *ShardWorker) updateSerialData(prevStart int64) int64 {
	output, err := shard.sharder.gce.GetSerialPortOutput(
		shard.sharder.projID, shard.zone, shard.name, prevStart)
	if err != nil {
		return prevStart
	}

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

func (shard *ShardWorker) shutdownOnTimeout(metadata *compute.Metadata) {
	shard.vmTimeout = true

	for _, item := range metadata.Items {
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

// finish calls gce-xfstests scripts to fetch and unpack test result files.
// It deletes the results in gs bucket and local serial port output on success.
func (shard *ShardWorker) finish(success bool) {
	if !success {
		shard.exit()
		return
	}
	url := shard.getResults()
	if url == "" {
		log.Printf("cannot find result file")
		shard.exit()
		return
	}
	file, err := os.OpenFile(shard.cmdLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	util.Check(err)
	defer util.Close(file)

	cmd := exec.Command("gce-xfstests", "get-results", "--unpack", url)
	log.Printf("%+v", cmd)
	err = util.CheckRun(cmd, util.RootDir, util.EmptyEnv, file, file)
	if err != nil {
		log.Printf("Get results failed with error: %s, args was: %s\n", err, cmd.String())
		shard.exit()
		return
	}

	if util.DirExists(shard.tmpResultsDir) {
		util.RemoveDir(shard.unpackedResultsDir)
		err = os.Rename(shard.tmpResultsDir, shard.unpackedResultsDir)
		util.Check(err)
	} else {
		log.Printf("results not found")
		shard.exit()
		return
	}

	if util.FileExists(shard.serialOutputPath) && !shard.vmTimeout {
		err = os.Remove(shard.serialOutputPath)
		util.Check(err)
	}

	prefix := fmt.Sprintf("%s/results.%s", shard.sharder.bucketSubdir, shard.resultsName)
	shard.sharder.gce.DeleteFiles(prefix)
	prefix = fmt.Sprintf("%s/summary.%s", shard.sharder.bucketSubdir, shard.resultsName)
	shard.sharder.gce.DeleteFiles(prefix)

}

func (shard *ShardWorker) getResults() string {
	attempts := 5
	prefix := fmt.Sprintf("%s/results.%s", shard.sharder.bucketSubdir, shard.resultsName)
	for attempts > 0 {
		resultFiles := shard.sharder.gce.GetFileNames(prefix)
		if len(resultFiles) == 1 {
			log.Printf("Found result file url: %v", resultFiles)
			return fmt.Sprintf("gs://%s/%s", shard.sharder.gsBucket, resultFiles[0])
		}
		attempts--
		time.Sleep(5 * time.Second)
	}
	return ""
}

// Info returns structured shard information to send back to user
func (shard *ShardWorker) Info(index int) ShardInfo {
	return ShardInfo{
		Index:   index,
		ShardID: shard.shardID,
		Config:  shard.config,
		Zone:    shard.zone,
	}
}

func (shard *ShardWorker) exit() {
	log.Printf("Existing shard gracefully with errors")
	if util.FileExists(shard.serialOutputPath) {
		log.Printf("Serial port output in results")
	} else {
		log.Printf("No results available")
	}
}
