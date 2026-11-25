/*
ShardWorker launches, monitors and collects results from a single
gce-xfstests run.

A shard is created and configured by a sharder only. It make a call to
the gce-xfstests scripts on start, and then waits for the test to
finish by checking the VM status periodically. After the test
finishes, the shard calls the scripts again to fetch the test result
files from GCS and unpacks them to a local directory.
*/
package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"thunk.org/gce-server/util/check"
	"thunk.org/gce-server/util/gcp"
	"thunk.org/gce-server/util/server"

	"github.com/sirupsen/logrus"
	"google.golang.org/api/compute/v1"
)

// ShardWorker manages a single test VM.
type ShardWorker struct {
	mu        sync.RWMutex
	sharder   *ShardScheduler
	shardID   string
	name      string
	zone      string
	config    string
	args      []string
	vmTimeout bool

	vmStatus       string
	vmtestStart    time.Time
	testResult     server.ResultType
	vmReset        bool
	vmTerminated   bool
	vmTermTime     time.Time
	vmTermInterval time.Duration

	log                *logrus.Entry
	logPath            string
	cmdLogPath         string
	serialOutputPath   string
	resultsName        string
	unpackedResultsDir string
}

const (
	resultsNoRebootsPath = "/usr/local/bin/results_no_reboots"
	noStatusTimeout      = 10 * time.Minute
	monitorInterval      = 60 * time.Second
	restartIntervalMin   = 1 * time.Minute
	restartIntervalMax   = 60 * time.Minute
	resetTimeout         = 10 * time.Minute
	gsInterval           = 10 * time.Second
	maxAttempts          = 5
)

// NewShardWorker constructs a new shard, requested by the sharder
func NewShardWorker(sharder *ShardScheduler, shardID string, config string, zone string) *ShardWorker {
	logPath := sharder.logDir + shardID
	shard := ShardWorker{
		sharder:   sharder,
		shardID:   shardID,
		name:      fmt.Sprintf("xfstests-ltm-%s-%s", sharder.testID, shardID),
		zone:      zone,
		config:    config,
		args:      []string{},
		vmTimeout: false,

		vmStatus:     "waiting for launch",
		vmtestStart:  time.Now(),
		testResult:   server.DefaultResult,
		vmReset:      false,
		vmTerminated: false,

		log:                sharder.log.WithField("shardID", shardID),
		logPath:            logPath,
		cmdLogPath:         logPath + ".cmdlog",
		serialOutputPath:   logPath + ".serial",
		resultsName:        fmt.Sprintf("%s-%s-%s", server.LTMUserName, sharder.testID, shardID),
		unpackedResultsDir: fmt.Sprintf("%sresults-%s-%s-%s", sharder.logDir, server.LTMUserName, sharder.testID, shardID),
	}

	shard.log.Info("Initializing test shard")

	shard.args = []string{
		"gce-xfstests",
		"--instance-name", shard.name,
		"--gce-zone", shard.zone,
		"--gs-bucket", sharder.gsBucket,
		"--kernel", sharder.gsKernel,
		"--bucket-subdir", sharder.bucketSubdir,
		"--no-email",
		"-c", config,
	}

	if sharder.arch != "" {
		shard.args = append(shard.args, "--arch", sharder.arch)
	}

	if sharder.kernelArch != "" {
		shard.args = append(shard.args, "--kernel-arch", sharder.kernelArch)
	}

	if !check.ContainsStr(sharder.validArgs, "--image-project") && len(sharder.imgProjID) > 0 {
		shard.args = append(shard.args, "--image-project", sharder.imgProjID)
	}

	// outside of ltm -> default is vms timeout
	// inside ltm -> default is no timeout (since ltm monitors vms and forces
	//               reboot and test skip if a test is crashed or stuck)
	// users can override default ltm no timeout by passing --vm-timeout flag
	// below sets no-vm-timeout by default if we do not explictly set anything
	if !check.ContainsStr(sharder.validArgs, "--vm-timeout") &&
			!check.ContainsStr(sharder.validArgs, "--no-vm-timeout"){
		shard.args = append(shard.args, "--no-vm-timeout")
	}

	shard.args = append(shard.args, sharder.validArgs...)

	return &shard
}

func (shard *ShardWorker) setStatus(status string) {
	shard.mu.Lock()
	shard.vmStatus = status
	shard.mu.Unlock()
}

func (shard *ShardWorker) getStatus() string {
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	return shard.vmStatus
}

func (shard *ShardWorker) setStart(t time.Time) {
	shard.mu.Lock()
	shard.vmtestStart = t
	shard.mu.Unlock()
}

func (shard *ShardWorker) getStart() time.Time {
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	return shard.vmtestStart
}

func (shard *ShardWorker) getResult() server.ResultType {
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	return shard.testResult
}

func (shard *ShardWorker) setStatusAndResult(status string, res server.ResultType) {
	shard.mu.Lock()
	shard.vmStatus = status
	shard.testResult = res
	shard.mu.Unlock()
}

// Run issues the gce-xfstests command to launch a test VM and monitor its running status.
func (shard *ShardWorker) Run(wg *sync.WaitGroup) {
	defer wg.Done()
	defer shard.exit()

	shard.log.WithField("shardInfo", shard.Info()).Debug("Starting shard")
	shard.setStatus("launching")

	file, err := os.Create(shard.cmdLogPath)
	check.Panic(err, shard.log, "Failed to create file")

	cmd := exec.Command(shard.args[0], shard.args[1:]...)
	shard.log.WithField("cmd", cmd.String()).Info("Launching test VM")
	err = check.LimitedRun(cmd, check.RootDir, check.EmptyEnv, file, file)
	file.Close()

	if err != nil {
		shard.log.WithError(err).WithField("cmd", cmd.String()).Error("Failed to start test VM")
		shard.setStatus("failed to launch")
	} else {
		shard.monitor()
		shard.finish()
	}
	shard.log.Info("Existing shard process")
}

/*
monitor blocks until the test VM finishes or timeout.

It queries the GCE api periodically and logs the serial console output
to a local file. If the VM no longer exists, stops running, or the
running test hasn't changed for more than monitorTimeout, the monitor
kills the test vm and returns.
*/
func (shard *ShardWorker) monitor() {
	shard.log.Info("Waiting for test VM to finish")

	var offset int64
	ticker := time.NewTicker(monitorInterval)
	defer ticker.Stop()
	monitorStart := time.Now()
	shard.setStart(monitorStart)

	for range ticker.C {
		log := shard.log.WithField("time", time.Since(monitorStart).Round(time.Second))
		instanceInfo, err := shard.sharder.gce.GetInstanceInfo(shard.sharder.projID, shard.zone, shard.name)

		if err != nil {
			if gcp.NotFound(err) {
				if shard.getStart() == monitorStart {
					shard.setStatus("failed to launch")
					log.Error("Test VM failed to launch")
				} else {
					log.Info("Test VM no longer exists")
				}
			} else {
				shard.setStatus("unexpected error")
				log.WithError(err).Panic("Failed to get shard instance info")
			}
			return
		}

		if !shard.vmTerminated {
			offset = shard.updateSerialData(offset)
		}

		if instanceInfo.Status == "TERMINATED" {
			if !shard.vmTerminated {
				shard.setStatus("terminated")
				shard.vmTerminated = true
				shard.vmTermTime = time.Now()
				shard.vmTermInterval = restartIntervalMin
				offset = 0
				log.Debug("VM Terminated")
				continue
			} else {
				if time.Since(shard.vmTermTime) >
					shard.vmTermInterval {
					shard.vmTermInterval *= 2
					if shard.vmTermInterval > restartIntervalMax {
						shard.vmTermInterval = restartIntervalMax
					}
					log.Debug("Restarting VM")
					err := shard.sharder.gce.StartVM(shard.sharder.projID, shard.zone, shard.name)
					check.NoError(err, log, "Failed to restart VM")
					shard.vmTermTime = time.Now()
				} else {
					log.WithFields(logrus.Fields{
						"interval": shard.vmTermInterval,
					}).Debug("Not yet time to restart")
				}
			}
		} else if instanceInfo.Status != "RUNNING" {
			log.WithFields(logrus.Fields{
				"instanceStatus": instanceInfo.Status,
			}).Debug("Unexpected VM instance status")
			continue
		} else {
			shard.vmTerminated = false
		}

		shard.applyInstanceMetadata(instanceInfo.Metadata, log)
		if shard.getStatus() == "launching" {
			if time.Since(monitorStart) > noStatusTimeout {
				if !shard.sharder.keepDeadVM {
					shard.shutdownOnTimeout(instanceInfo.Metadata)
				}
				shard.setStatusAndResult("timeout without launching tests", server.Error)

				log.Errorf("Tests might fail to start, cannot find test status for %s", noStatusTimeout.Round(time.Second))
				return
			}
			log.Debug("waiting to get test status metadata")
		}

		if shard.vmReset && time.Since(shard.getStart()) > resetTimeout {
			log.Errorf("VM did not come back online after reset, exiting")
			return
		}

		// Reset VM if we don't get a status update
		// Skip check if we are already performing a reset
		// Selftests may limit monitorTimeout to shorter than noStatusTimeout
		//    so skip check if we are still launching
		if time.Since(shard.getStart()) > shard.sharder.monitorTimeout &&
			!shard.vmReset && shard.getStatus() != "launching" {
			log.Debug("Resetting VM")
			err := shard.sharder.gce.ResetVM(shard.sharder.projID, shard.zone, shard.name)
			if err != nil {
				log.Errorf("Failed to reset %s", shard.name)
				shard.setStatusAndResult("failed to reset after timeout", server.Error)
				return
			}
			shard.vmReset = true
			shard.setStart(time.Now())
		}

		shard.mu.RLock()
		curStatus := shard.vmStatus
		curStart := shard.vmtestStart
		shard.mu.RUnlock()
		log.WithFields(logrus.Fields{
			"status": curStatus,
			"start":  curStart.Format(time.Stamp),
		}).Debug("Keep waiting")
	}
}

// updateSerialData writes the serial port output from the test VM to local log file.
func (shard *ShardWorker) updateSerialData(offset int64) int64 {
	log := shard.log.WithField("offset", offset)
	output, err := shard.sharder.gce.GetSerialPortOutput(
		shard.sharder.projID, shard.zone, shard.name, offset)
	if err != nil {
		log.Debug("Unable to get serial output, VM might be down")
		return offset
	}

	file, err := os.OpenFile(shard.serialOutputPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if !check.NoError(err, log, "Failed to open file") {
		return offset
	}
	defer file.Close()

	if output.Start > offset {
		log.WithField("newStart", output.Start).Info("Missing some serial port output")
		_, err := file.WriteString(fmt.Sprintf(
			"\n!=====Missing data from %d to %d=====!\n", offset, output.Start))
		if !check.NoError(err, log, "Failed to write file") {
			return offset
		}
	}
	log.Debug("Writing serial port output to file")
	_, err = file.WriteString(output.Contents)
	if !check.NoError(err, log, "Failed to write file") {
		return offset
	}

	return output.Next
}

func parseMetadataKernelVersion(raw string) (string, bool) {
	fields := strings.Fields(raw)
	var ver string
	if len(fields) >= 3 && fields[0] == "Linux" {
		ver = fields[2]
	} else if len(fields) == 1 {
		ver = fields[0]
	} else {
		return "", false
	}
	if err := check.ValidateTestID(ver); err != nil {
		return "", false
	}
	return ver, true
}

func (shard *ShardWorker) applyInstanceMetadata(metadata *compute.Metadata, log *logrus.Entry) {
	if metadata == nil {
		return
	}
	for _, metaData := range metadata.Items {
		if metaData == nil || metaData.Value == nil {
			continue
		}
		switch metaData.Key {
		case "status":
			shard.mu.Lock()
			if *metaData.Value != shard.vmStatus {
				shard.vmStatus = *metaData.Value
				shard.vmtestStart = time.Now()
				shard.vmReset = false
			}
			shard.mu.Unlock()
		case "kernel_version":
			if ver, ok := parseMetadataKernelVersion(*metaData.Value); ok {
				if shard.sharder.setKernelVersionIfUnknown(ver) && log != nil {
					log.WithField("kernelVersion", ver).Info("Updated kernel version from shard metadata")
				}
			}
		}
	}
}

func (shard *ShardWorker) shutdownOnTimeout(metadata *compute.Metadata) {
	shard.log.Info("Shutting down")
	shard.mu.Lock()
	shard.vmTimeout = true
	shard.mu.Unlock()

	for _, item := range metadata.Items {
		if item.Key == "shutdown_reason" {
			return
		}
	}

	val := "LTM detected test timeout"
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

/*
finish calls gce-xfstests scripts to fetch and unpack test result files.
It deletes the results in gs bucket and determines testResult:

Default		VM finishes without issues, test result is found;
Crash		VM started running tests but no test result is found;
Hang		VM stays on one test for too long;
Error		VM stops at launch time, doesn't launch any tests at all,

	or other unexpected errors.
*/
func (shard *ShardWorker) finish() {
	shard.log.Info("Finishing shard")

	url := shard.getResults()
	if url == "" {
		shard.mu.Lock()
		if shard.testResult == server.DefaultResult {
			if shard.vmStatus == "launching" {
				shard.testResult = server.Error
				shard.vmStatus = "finished without launching tests"
			} else {
				shard.testResult = server.Crash
			}
		}
		shard.mu.Unlock()
		shard.log.Error("Failed to find result file")
		return
	}

	cmd := exec.Command("gce-xfstests", "get-results", "--unpack-dir",
		shard.sharder.logDir, url)
	cmdLog := shard.log.WithField("cmd", cmd.String())
	w := cmdLog.Writer()
	defer w.Close()
	err := check.LimitedRun(cmd, check.RootDir, check.EmptyEnv, w, w)
	check.Panic(err, cmdLog, "Failed to run get-results")

	if !check.DirExists(shard.unpackedResultsDir) {
		shard.log.Panic("Failed to find unpacked result files")
	}

	cmd = exec.Command(resultsNoRebootsPath, shard.unpackedResultsDir)
	err = cmd.Run()
	check.NoError(err, shard.log, "Failed to check for VM reboots")
	shard.mu.RLock()
	timedOut := shard.vmTimeout
	shard.mu.RUnlock()
	if err == nil && !timedOut &&
		check.FileExists(shard.serialOutputPath) {
		err = os.Remove(shard.serialOutputPath)
		check.NoError(err, shard.log, "Failed to remove dir")
	}

	prefix := fmt.Sprintf("%s/results.%s", shard.sharder.bucketSubdir, shard.resultsName)
	_, err = shard.sharder.gce.DeleteFiles(prefix)
	check.NoError(err, shard.log, "Failed to delete file")

	prefix = fmt.Sprintf("%s/summary.%s", shard.sharder.bucketSubdir, shard.resultsName)
	_, err = shard.sharder.gce.DeleteFiles(prefix)
	check.NoError(err, shard.log, "Failed to delete file")
	shard.setStatus("finished")
}

// getResults fetches the test result files.
// It returns empty string if cannot find the result file in maxAttempts.
func (shard *ShardWorker) getResults() string {
	shard.log.Info("Fetching test results")
	prefix := fmt.Sprintf("%s/results.%s", shard.sharder.bucketSubdir, shard.resultsName)
	for attempts := maxAttempts; attempts > 0; attempts-- {
		resultFiles, err := shard.sharder.gce.GetFileNames(prefix)
		check.NoError(err, shard.log, "Failed to get GS filenames")
		resultURL := check.SelectResultTarball(shard.sharder.gsBucket, resultFiles)
		if err == nil && resultURL != "" {
			shard.log.WithField("resultURL", resultURL).Info("Found result file url")
			return resultURL

		}
		shard.log.WithFields(logrus.Fields{
			"attemptsLeft": attempts,
			"prefix":       prefix,
			"resultURL":    resultURL,
			"err":          err,
		}).Debug("No GS file with matching url")
		time.Sleep(gsInterval)
	}
	return ""
}

// Info returns structured shard information.
func (shard *ShardWorker) Info() server.ShardInfo {
	shard.mu.RLock()
	status := shard.vmStatus
	start := shard.vmtestStart
	result := shard.testResult
	shard.mu.RUnlock()

	return server.ShardInfo{
		ID:     shard.shardID,
		Config: shard.config,
		Zone:   shard.zone,
		Status: status,
		Time:   time.Since(start).Round(time.Second).String(),
		Result: result.String(),
	}
}

// exit handles panic from shard run.
func (shard *ShardWorker) exit() {
	if r := recover(); r != nil {
		shard.log.Error("Shard exits with error, get stack trace")
		shard.log.Error(string(debug.Stack()))
		if check.FileExists(shard.serialOutputPath) {
			shard.log.Warn("Serial port output is found")
		} else {
			shard.log.Warn("Serial port output is not found")
		}
		shard.mu.Lock()
		if shard.testResult == server.DefaultResult {
			shard.testResult = server.Error
		}
		shard.mu.Unlock()
	}
}
