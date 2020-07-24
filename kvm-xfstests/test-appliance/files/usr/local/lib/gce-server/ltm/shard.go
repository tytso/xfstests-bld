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
	"os"
	"os/exec"
	"sync"
	"time"

	"gce-server/util/check"
	"gce-server/util/gcp"

	"github.com/sirupsen/logrus"
	"google.golang.org/api/compute/v1"
)

// ShardWorker manages a single test VM.
type ShardWorker struct {
	sharder   *ShardSchedular
	shardID   string
	name      string
	zone      string
	config    string
	args      []string
	vmTimeout bool

	log                *logrus.Entry
	logPath            string
	cmdLogPath         string
	serialOutputPath   string
	resultsName        string
	tmpResultsDir      string
	unpackedResultsDir string
}

// ShardInfo exports shard info to be sent back to user.
type ShardInfo struct {
	ShardID string `json:"shard_id"`
	Config  string `json:"cfg"`
	Zone    string `json:"zone"`
}

// NewShardWorker constructs a new shard, requested by the sharder
func NewShardWorker(sharder *ShardSchedular, shardID string, config string, zone string) *ShardWorker {
	logPath := sharder.logDir + shardID
	shard := ShardWorker{
		sharder:   sharder,
		shardID:   shardID,
		name:      fmt.Sprintf("xfstests-ltm-%s-%s", sharder.testID, shardID),
		zone:      zone,
		config:    config,
		args:      []string{},
		vmTimeout: false,

		log:                sharder.log.WithField("shardID", shardID),
		logPath:            logPath,
		cmdLogPath:         logPath + ".cmdlog",
		serialOutputPath:   logPath + ".serial",
		resultsName:        fmt.Sprintf("%s-%s-%s", LTMUserName, sharder.testID, shardID),
		tmpResultsDir:      fmt.Sprintf("/tmp/results-%s-%s-%s", LTMUserName, sharder.testID, shardID),
		unpackedResultsDir: fmt.Sprintf("%sresults-%s-%s-%s", sharder.logDir, LTMUserName, sharder.testID, shardID),
	}

	shard.log.Info("Initializing test shard")

	shard.args = []string{
		"gce-xfstests",
		"--instance-name", shard.name,
		"--gce-zone", shard.zone,
		"--gs-bucket", sharder.gsBucket,
		"--image-project", sharder.projID,
		"--kernel", sharder.gsKernel,
		"--bucket-subdir", sharder.bucketSubdir,
		"--no-email",
		"-c", config,
	}
	shard.args = append(shard.args, sharder.validArgs...)

	return &shard
}

// Run issues the gce-xfstests command to launch a test vm and monitor its running status
func (shard *ShardWorker) Run(wg *sync.WaitGroup) {
	defer wg.Done()

	// handle exceptions (panic) here
	defer shard.exit()

	shard.log.WithField("shardInfo", shard.Info()).Debug("Starting shard")

	file, err := os.Create(shard.cmdLogPath)
	check.Panic(err, shard.log, "Failed to create file")

	cmd := exec.Command(shard.args[0], shard.args[1:]...)
	shard.log.WithField("cmd", cmd.String()).Info("Launching test VM")
	err = check.Run(cmd, check.RootDir, check.EmptyEnv, file, file)
	file.Close()

	if err != nil {
		shard.log.WithError(err).WithField("cmd", cmd.String()).Error("Failed to start test VM")
	} else {
		shard.monitor()
		shard.finish()
	}
	shard.log.Info("Existing shard process")
}

// monitor queries the GCE api every minute and logs the serial console output
// to a local file. If the vm no longer exists or the status hasn't changed for
// more than an hour, the monitor kills the test vm.
// panic if VM doesn't finish within 1 hour
func (shard *ShardWorker) monitor() {
	var (
		waitTime       int
		timePrevStatus int
		prevStart      int64
		prevStatus     string
	)
	shard.log.Info("Waiting for test VM to finish")

	for true {
		time.Sleep(60 * time.Second)
		waitTime += 60
		log := shard.log.WithField("waited", waitTime)
		instanceInfo, err := shard.sharder.gce.GetInstanceInfo(shard.sharder.projID, shard.zone, shard.name)

		if err != nil {
			if gcp.NotFound(err) {
				// If prevStatus is empty, it's likely the VM never launched
				if prevStatus == "" {
					log.Panic("Test VM failed to launch")
				} else {
					log.Info("Test VM no longer exists")
				}
				break
			}
			log.WithError(err).Panic("Failed to get instance info")
		}

		prevStart = shard.updateSerialData(prevStart)

		if instanceInfo.Status != prevStatus {
			timePrevStatus = waitTime
			prevStatus = instanceInfo.Status
		} else if waitTime > timePrevStatus+3600 {
			if !shard.sharder.keepDeadVM {
				shard.shutdownOnTimeout(instanceInfo.Metadata)
			}
			// TODO: validate this step (i.e. whether we wait fot the VM to be deleted)
			log.WithFields(logrus.Fields{
				"prevStatus":     prevStatus,
				"timePrevStatus": timePrevStatus,
			}).Panic("Instance seems to have wedged, no status update for 1 hour")
		}

		log.WithFields(logrus.Fields{
			"prevStatus":     prevStatus,
			"timePrevStatus": timePrevStatus,
		}).Debug("Keep waiting")
	}
}

// updateSerialData writes the serial port output from the test vm to local log file.
func (shard *ShardWorker) updateSerialData(prevStart int64) int64 {
	log := shard.log.WithField("prevStart", prevStart)
	output, err := shard.sharder.gce.GetSerialPortOutput(
		shard.sharder.projID, shard.zone, shard.name, prevStart)
	if err != nil {
		log.Debug("Unable to get serial output, VM might be down")
		return prevStart
	}

	file, err := os.OpenFile(shard.serialOutputPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if !check.NoError(err, log, "Failed to open file") {
		return prevStart
	}
	defer file.Close()

	if output.Start > prevStart {
		log.WithField("newStart", output.Start).Info("Missing some serial port output")
		_, err := file.WriteString(fmt.Sprintf(
			"\n!=====Missing data from %d to %d=====!\n", prevStart, output.Start))
		if !check.NoError(err, log, "Failed to write file") {
			return prevStart
		}
	}
	log.Debug("Writing serial port output to file")
	_, err = file.WriteString(output.Contents)
	if !check.NoError(err, log, "Failed to write file") {
		return prevStart
	}

	return output.Next
}

func (shard *ShardWorker) shutdownOnTimeout(metadata *compute.Metadata) {
	shard.log.Info("Shutting down")
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

	err := shard.sharder.gce.SetMetadata(shard.sharder.projID, shard.zone, shard.name, metadata)
	check.NoError(err, shard.log, "Failed to set VM metadata")
	// allow some time for metadata to be set
	time.Sleep(1 * time.Second)
	err = shard.sharder.gce.DeleteInstance(shard.sharder.projID, shard.zone, shard.name)
	check.NoError(err, shard.log, "Failed to delete VM")
}

// finish calls gce-xfstests scripts to fetch and unpack test result files.
// It deletes the results in gs bucket and local serial port output.
func (shard *ShardWorker) finish() {
	shard.log.Info("Finishing shard")

	url := shard.getResults()
	if url == "" {
		shard.log.Panic("Failed to find result file")
	}

	cmd := exec.Command("gce-xfstests", "get-results", "--unpack", url)
	cmdLog := shard.log.WithField("cmd", cmd.String())
	w := cmdLog.Writer()
	defer w.Close()
	err := check.Run(cmd, check.RootDir, check.EmptyEnv, w, w)
	check.Panic(err, cmdLog, "Failed to run get-results")

	if check.DirExists(shard.tmpResultsDir) {
		check.RemoveDir(shard.unpackedResultsDir)
		err = os.Rename(shard.tmpResultsDir, shard.unpackedResultsDir)
		check.Panic(err, shard.log, "Failed to move dir")
	} else {
		shard.log.Panic("Failed to find unpacked result files")
	}

	if check.FileExists(shard.serialOutputPath) && !shard.vmTimeout {
		err = os.Remove(shard.serialOutputPath)
		check.NoError(err, shard.log, "Failed to remove dir")
	}

	prefix := fmt.Sprintf("%s/results.%s", shard.sharder.bucketSubdir, shard.resultsName)
	_, err = shard.sharder.gce.DeleteFiles(prefix)
	check.NoError(err, shard.log, "Failed to delete file")

	prefix = fmt.Sprintf("%s/summary.%s", shard.sharder.bucketSubdir, shard.resultsName)
	_, err = shard.sharder.gce.DeleteFiles(prefix)
	check.NoError(err, shard.log, "Failed to delete file")
}

// getResults fetches the test result files.
// return "" if cannot find the result file in 5 attempts
func (shard *ShardWorker) getResults() string {
	shard.log.Info("Fetching test results")
	attempts := 5
	prefix := fmt.Sprintf("%s/results.%s", shard.sharder.bucketSubdir, shard.resultsName)
	for attempts > 0 {
		resultFiles, err := shard.sharder.gce.GetFileNames(prefix)
		check.NoError(err, shard.log, "Failed to get GS filenames")
		if err == nil && len(resultFiles) == 1 {
			shard.log.WithField("resultURL", resultFiles[0]).Info("Found result file url")
			return fmt.Sprintf("gs://%s/%s", shard.sharder.gsBucket, resultFiles[0])
		}
		attempts--
		shard.log.WithField("attemptsLeft", attempts).Debug("No GS file with matching url")
		time.Sleep(5 * time.Second)
	}
	return ""
}

// Info returns structured shard information to send back to user
func (shard *ShardWorker) Info() ShardInfo {
	return ShardInfo{
		ShardID: shard.shardID,
		Config:  shard.config,
		Zone:    shard.zone,
	}
}

// exit handles panic from shard run.
func (shard *ShardWorker) exit() {
	if r := recover(); r != nil {
		shard.log.WithField("panic", r).Warn("Shard finishes with errors, regular result files are not available")
		if check.FileExists(shard.serialOutputPath) {
			shard.log.Warn("Serial port output is found")
		} else {
			shard.log.Warn("Serial port output is not found")
		}
	}
}
