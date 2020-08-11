/*
ShardScheduler arranges the tests and runs them in multiple shardWorkers.

The sharder parses the command line arguments sent by user, parse it into
machine understandable xfstests configs. Then it queries for GCE quotas and
spawns a suitable number of shards to run the tests. The sharder waits until
all shards finish, fetch the result files and aggregate them. An email is sent
to the user if necessary.

The TestRunManager from previous flask version is integrated into shardScheduler
now to reduce the code base.

*/
package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"gce-server/util/check"
	"gce-server/util/email"
	"gce-server/util/gcp"
	"gce-server/util/logging"
	"gce-server/util/mymath"
	"gce-server/util/parser"
	"gce-server/util/server"

	"github.com/sirupsen/logrus"
)

const genResultsSummaryPath = "/usr/local/bin/gen_results_summary"

// ShardScheduler schedules tests and aggregates reports.
type ShardScheduler struct {
	testID  string
	projID  string
	origCmd string

	zone           string
	region         string
	gsBucket       string
	bucketSubdir   string
	gsKernel       string
	kernelVersion  string
	reportReceiver string
	maxShards      int
	keepDeadVM     bool

	reportKCS   bool
	testRequest server.TaskRequest
	testResult  server.ResultType

	log     *logrus.Entry
	logDir  string
	logFile string
	aggDir  string
	aggFile string

	validArgs []string
	configs   []string
	gce       *gcp.Service
	shards    []*ShardWorker
}

// SharderInfo exports sharder info.
type SharderInfo struct {
	ID        string      `json:"id"`
	NumShards int         `json:"num_shards"`
	ShardInfo []ShardInfo `json:"shard_info"`
}

// NewShardScheduler constructs a new sharder from a test request.
// All dir strings have a trailing / for consistency purpose,
// except for bucketSubdir.
func NewShardScheduler(c server.TaskRequest, testID string) *ShardScheduler {
	logDir := logging.LTMLogDir + testID + "/"
	err := check.CreateDir(logDir)
	if err != nil {
		panic(err)
	}

	logFile := logDir + "run.log"
	log := logging.InitLogger(logFile)

	data, err := base64.StdEncoding.DecodeString(c.CmdLine)
	check.Panic(err, log, "Failed to decode cmdline")

	// assume a zone looks like us-central1-f and a region looks like us-central1
	// syntax might change in the future so should add support to query for it
	zone, err := gcp.GceConfig.Get("GCE_ZONE")
	check.Panic(err, log, "Failed to get zone config")
	region := zone[:len(zone)-2]

	projID, err := gcp.GceConfig.Get("GCE_PROJECT")
	check.Panic(err, log, "Failed to get project config")

	gsBucket, err := gcp.GceConfig.Get("GS_BUCKET")
	check.Panic(err, log, "Failed to get gs bucket config")

	bucketSubdir, _ := gcp.GceConfig.Get("BUCKET_SUBDIR")

	log.Info("Initiating test sharder")
	sharder := ShardScheduler{
		testID:  testID,
		projID:  projID,
		origCmd: strings.TrimSpace(string(data)),

		zone:           zone,
		region:         region,
		gsBucket:       gsBucket,
		bucketSubdir:   bucketSubdir,
		gsKernel:       c.Options.GsKernel,
		kernelVersion:  "unknown_kernel_version",
		reportReceiver: c.Options.ReportEmail,
		maxShards:      0,
		keepDeadVM:     false,

		reportKCS:   false,
		testRequest: c,
		testResult:  server.UnknownResult,

		log:     log,
		logDir:  logDir,
		logFile: logFile,
		aggDir:  fmt.Sprintf("%sresults-%s-%s/", logDir, LTMUserName, testID),
		aggFile: fmt.Sprintf("%sresults.%s-%s", logDir, LTMUserName, testID),
	}

	if _, err := gcp.GceConfig.Get("GCE_LTM_KEEP_DEAD_VM"); err == nil {
		sharder.keepDeadVM = true
	}
	if c.Options.BucketSubdir != "" {
		sharder.bucketSubdir = c.Options.BucketSubdir
	}
	if sharder.bucketSubdir == "" {
		sharder.bucketSubdir = "results"
	}

	sharder.validArgs, sharder.configs, err = getConfigs(sharder.origCmd)
	check.Panic(err, log, "Failed to parse config from origCmd")

	sharder.gce, err = gcp.NewService(sharder.gsBucket)
	check.Panic(err, log, "Failed to connect to GCE service")

	regionShard := !c.Options.NoRegionShard
	if regionShard {
		sharder.initRegionSharding()
	} else {
		sharder.initLocalSharding()
	}

	if c.ExtraOptions != nil && c.ExtraOptions.Requester == server.KCSBisectStep {
		sharder.reportKCS = true
	}

	return &sharder
}

// initLocalSharding creates shards in the same zone the VM runs in.
// The sharder queries for available quotas in the current zone and
// spawns new shards accordingly.
func (sharder *ShardScheduler) initLocalSharding() {
	log := sharder.log.WithField("region", sharder.region)
	log.Info("Initilizing local sharding")
	allShards := []*ShardWorker{}
	quota, err := sharder.gce.GetRegionQuota(sharder.projID, sharder.region)
	check.Panic(err, log, "Failed to get quota")

	if quota == nil {
		log.Panic("GCE region is out of quota")
	}
	numShards, err := quota.GetMaxShard()
	check.Panic(err, log, "Failed to get max shard")

	if sharder.maxShards > 0 {
		numShards = mymath.MaxInt(numShards, sharder.maxShards)
	}
	configs := splitConfigs(numShards, sharder.configs)

	for i, config := range configs {
		shardID := string(i/26+int('a')) + string(i%26+int('a'))
		shard := NewShardWorker(sharder, shardID, config, sharder.zone)
		allShards = append(allShards, shard)
	}

	sharder.shards = allShards
}

// initRegionSharding creates shards among all zones with available quotas.
// It first query all zones on the same continent as the project, and queries
// other zones if the quota is not enough to assign each config to a separate VM.
func (sharder *ShardScheduler) initRegionSharding() {
	continent := strings.Split(sharder.region, "-")[0]
	log := sharder.log.WithField("continent", continent)
	log.Info("Initilizing region sharding")

	allShards := []*ShardWorker{}
	quotas, err := sharder.gce.GetAllRegionsQuota(sharder.projID)
	check.Panic(err, log, "Failed to get quota")

	usedZones := []string{}

	for _, quota := range quotas {
		if strings.HasPrefix(quota.Zone, continent) {
			maxShard, err := quota.GetMaxShard()
			check.Panic(err, log, "Failed to get max shard")

			for i := 0; i < maxShard; i++ {
				usedZones = append(usedZones, quota.Zone)
			}
		}
	}
	rand.Shuffle(len(usedZones), func(i, j int) {
		usedZones[i], usedZones[j] = usedZones[j], usedZones[i]
	})

	if len(usedZones) < len(sharder.configs) {
		for _, quota := range quotas {
			if !strings.HasPrefix(quota.Zone, continent) {
				maxShard, err := quota.GetMaxShard()
				check.Panic(err, log, "Failed to get max shard")

				for i := 0; i < maxShard; i++ {
					usedZones = append(usedZones, quota.Zone)
				}
			}
			if len(usedZones) >= len(sharder.configs) {
				break
			}
		}
	}
	if len(usedZones) == 0 {
		log.WithField("projID", sharder.projID).Panic("GCE project is out of quota")
	}
	configs := splitConfigs(len(usedZones), sharder.configs)

	for i, config := range configs {
		shardID := string(i/26+int('a')) + string(i%26+int('a'))
		shard := NewShardWorker(sharder, shardID, config, usedZones[i])
		allShards = append(allShards, shard)
	}

	sharder.shards = allShards
}

// getConfigs calls a parser to extract the valid args and configs from
// the raw cmdline.
func getConfigs(origCmd string) ([]string, []string, error) {
	validArgs, configs, err := parser.Cmd(origCmd)
	if err != nil {
		return []string{}, []string{}, nil
	}
	configStrings := []string{}
	for fs := range configs {
		for _, cfg := range configs[fs] {
			if cfg != "dax" {
				configStrings = append(configStrings, fs+"/"+cfg)
			}
		}
	}
	return validArgs, configStrings, nil
}

// splitConfigs distribute configs among shards in a round-robin way.
func splitConfigs(numShards int, configs []string) []string {
	if numShards <= 0 || len(configs) <= numShards {
		return configs
	}

	configGroups := make([][]string, numShards)
	idx := 0
	for _, config := range configs {
		configGroups[idx] = append(configGroups[idx], config)
		idx = (idx + 1) % numShards
	}
	configConcat := make([]string, numShards)
	for i, group := range configGroups {
		configConcat[i] = strings.Join(group, ",")
	}
	return configConcat
}

// Run starts all the shards in a separate go routine.
// Wait between starting shards to avoid hitting the api too hard.
func (sharder *ShardScheduler) Run() {
	sharder.log.Debug("Starting sharder")
	var wg sync.WaitGroup

	subject := fmt.Sprintf("xfstests failure %s-%s %s", LTMUserName, sharder.testID, sharder.kernelVersion)
	defer email.ReportFailure(sharder.log, sharder.logFile, sharder.reportReceiver, subject)

	defer sharder.clean()

	if sharder.reportKCS {
		defer sharder.sendKCSReport()
	}

	for _, shard := range sharder.shards {
		wg.Add(1)
		go shard.Run(&wg)
		time.Sleep(500 * time.Millisecond)
	}
	wg.Wait()

	sharder.log.Debug("All shards finished")
	sharder.finish()
}

// Info returns structured sharder information.
func (sharder *ShardScheduler) Info() SharderInfo {
	info := SharderInfo{
		ID:        sharder.testID,
		NumShards: len(sharder.shards),
	}

	for _, shard := range sharder.shards {
		info.ShardInfo = append(info.ShardInfo, shard.Info())
	}

	return info
}

// aggregate results and upload a tarball to gs bucket.
// panic and send an email to user if no results available.
func (sharder *ShardScheduler) finish() {
	sharder.log.Debug("Finishing sharder")

	sharder.aggResults()
	sharder.createInfo()
	sharder.createRunStats()
	sharder.genResultsSummary()

	if !sharder.reportKCS {
		sharder.emailReport()
	}

	sharder.packResults()
}

// aggResults looks for results file from each shard and aggregates them.
func (sharder *ShardScheduler) aggResults() {
	err := check.CreateDir(sharder.aggDir)
	check.Panic(err, sharder.log, "Failed to create dir")

	hasResults := false
	for _, shard := range sharder.shards {
		log := sharder.log.WithFields(logrus.Fields{
			"shardID":            shard.shardID,
			"unpackedResultsDir": shard.unpackedResultsDir,
		})
		log.Debug("Moving shard result files into aggregate folder")

		if check.DirExists(shard.unpackedResultsDir) {
			err := os.RemoveAll(sharder.aggDir + shard.shardID)
			check.Panic(err, log, "Failed to remove dir")

			err = os.Rename(shard.unpackedResultsDir, sharder.aggDir+shard.shardID)
			check.Panic(err, log, "Failed to move dir")

			hasResults = true
		} else if check.FileExists(shard.serialOutputPath) {
			err := os.RemoveAll(sharder.aggDir + shard.shardID + ".serial")
			check.Panic(err, log, "Failed to remove dir")

			err = os.Rename(shard.serialOutputPath, sharder.aggDir+shard.shardID+".serial")
			check.Panic(err, log, "Failed to move dir")

			hasResults = true
		} else {
			log.Warn("Shard has no results available")
		}
	}
	if !hasResults {
		sharder.log.Error("No shard created any results or serial dumps before exiting")
		sharder.log.Panic("No results available for any of the shards")
	}

	for _, config := range []string{"runtests.log", "cmdline", "summary", "failures", "run-stats",
		"testrunid", "kernel_version"} {
		sharder.concatResults(config)
	}

	for _, shard := range sharder.shards {
		kernelVersionFile := fmt.Sprintf("%s%s/kernel_version", sharder.aggDir, shard.shardID)
		if check.FileExists(kernelVersionFile) {
			content, err := check.ReadLines(kernelVersionFile)
			if !check.NoError(err, sharder.log, "Failed to read file") {
				continue
			}
			sharder.kernelVersion = content[0]
		}
	}
}

// concatResults aggregate all shard files of a given file type by producing
// a concatenated file at the top level of the aggregate results directory.
func (sharder *ShardScheduler) concatResults(filename string) {
	log := sharder.log.WithField("resultFile", filename)
	log.Info("Cancatenating shard result file")

	file, err := os.Create(sharder.aggDir + filename)
	check.Panic(err, log, "Failed to create file")

	defer file.Close()

	fmt.Fprintf(file, "LTM aggregate file for %s\n", filename)
	fmt.Fprintf(file, "Test run ID %s\n", sharder.testID)
	fmt.Fprintf(file, "Aggregate results from %d shards\n", len(sharder.shards))

	for _, shard := range sharder.shards {
		shardLog := log.WithField("shardID", shard.shardID)
		fmt.Fprintf(file, "\n============SHARD %s============\n", shard.shardID)
		fmt.Fprintf(file, "============CONFIG: %s\n\n", shard.config)
		shardFile := fmt.Sprintf("%s%s/%s", sharder.aggDir, shard.shardID, filename)
		if check.FileExists(shardFile) {
			sourceFile, err := os.Open(shardFile)
			if check.NoError(err, shardLog, "Failed to open file") {
				_, err = io.Copy(file, sourceFile)
				check.NoError(err, shardLog, "Failed to copy file")

				sourceFile.Close()
			}
		} else {
			shardLog.Warn("Failed to find shard result file")
			fmt.Fprintf(file, "Could not open/read file %s for shard %s\n", filename, shard.shardID)
		}
		fmt.Fprintf(file, "\n==========END SHARD %s==========\n", shard.shardID)
	}
}

// createInfo creates an ltm-info file and an ltm_logs directory.
func (sharder *ShardScheduler) createInfo() {
	sharder.log.Info("Creating LTM info")
	ltmLogDir := sharder.aggDir + "ltm_logs/"
	err := check.CreateDir(ltmLogDir)
	if !check.NoError(err, sharder.log, "Failed to create dir") {
		return
	}

	file, err := os.Create(sharder.aggDir + "ltm-info")
	if !check.NoError(err, sharder.log, "Failed to create file") {
		return
	}

	defer file.Close()

	fmt.Fprintf(file, "LTM test run ID %s\n", sharder.testID)
	fmt.Fprintf(file, "Original command: %s\n", sharder.origCmd)
	fmt.Fprintf(file, "Aggregate results from %d shards\n", len(sharder.shards))
	fmt.Fprint(file, "SHARD INFO:\n\n")

	for _, shard := range sharder.shards {
		fmt.Fprintf(file, "SHARD %s\n", shard.shardID)
		fmt.Fprintf(file, "instance name: %s\n", shard.name)
		fmt.Fprintf(file, "split config: %s\n", shard.config)
		fmt.Fprintf(file, "gce command executed: %v\n\n", shard.args)
	}

	sharder.log.Info("Finished creating ltm-info")
}

func (sharder *ShardScheduler) createRunStats() {
	sharder.log.Info("Creating LTM run stats")
	file, err := os.Create(sharder.aggDir + "ltm-run-stats")
	if err != nil {
		sharder.log.Error("Failed to create file")
		return
	}
	defer file.Close()

	fmt.Fprintf(file, "TESTRUNID: %s-%s\n", LTMUserName, sharder.testID)
	fmt.Fprintf(file, "CMDLINE: %s\n", sharder.origCmd)

}

// genResultsSummary calls a python script to generate the summary on junit xml test results.
// It parses the summary file to check for any test failures.
func (sharder *ShardScheduler) genResultsSummary() {
	cmd := exec.Command(genResultsSummaryPath, sharder.aggDir, "--output_file", sharder.aggDir+"report")
	cmdLog := sharder.log.WithField("cmd", cmd.String())
	w := cmdLog.Writer()
	defer w.Close()
	err := check.LimitedRun(cmd, check.RootDir, check.EmptyEnv, w, w)
	check.NoError(err, cmdLog, "Failed to run python script gen_results_summary")

	content, err := ioutil.ReadFile(sharder.aggDir + "report")
	if check.NoError(err, sharder.log, "Failed to read the report file") {
		if strings.Contains(string(content), "0 failures") {
			sharder.testResult = server.Pass
		} else {
			sharder.testResult = server.Failure
		}
	}
}

// emailReport sends the email
func (sharder *ShardScheduler) emailReport() {
	sharder.log.Info("Sending email report")
	subject := fmt.Sprintf("xfstests results %s-%s %s", LTMUserName, sharder.testID, sharder.kernelVersion)

	b, err := ioutil.ReadFile(sharder.aggDir + "report")
	content := string(b)
	if !check.NoError(err, sharder.log, "Failed to read the report file") {
		content = "Unable to generate test summary report"
	}

	err = email.Send(subject, content, sharder.reportReceiver)
	check.Panic(err, sharder.log, "Failed to send the email")
}

func (sharder *ShardScheduler) sendKCSReport() {
	sharder.testRequest.ExtraOptions.TestID = strings.Split(sharder.testID, "-")[0]
	sharder.testRequest.ExtraOptions.TestResult = sharder.testResult
	sharder.testRequest.ExtraOptions.Requester = server.LTMBisectStep

	server.SendInternalRequest(sharder.testRequest, sharder.log, true)
}

// packResults packs the aggregared files after copying the sharder's log file into it.
func (sharder *ShardScheduler) packResults() {
	sharder.log.Info("Packing aggregated files")
	sharder.log.Info("Copying sharder log file")

	logging.Sync(sharder.log)
	aggLogFile := sharder.aggDir + "ltm_logs/run.log"
	err := check.CopyFile(aggLogFile, sharder.logFile)
	if err != nil {
		check.Panic(err, sharder.log, "Failed to copy sharder log file")
	}

	cmd := exec.Command("tar", "-cf", sharder.aggFile+".tar", "-C", sharder.aggDir, ".")
	cmdLog := sharder.log.WithField("cmd", cmd.Args)
	w1 := cmdLog.Writer()
	defer w1.Close()
	err = check.Run(cmd, check.RootDir, check.EmptyEnv, w1, w1)
	if !check.NoError(err, cmdLog, "Failed to create tarball") {
		return
	}

	cmd = exec.Command("xz", "-6ef", sharder.aggFile+".tar")
	cmdLog = sharder.log.WithField("cmd", cmd.Args)
	w2 := cmdLog.Writer()
	defer w2.Close()
	err = check.Run(cmd, check.RootDir, check.EmptyEnv, w2, w2)
	if !check.NoError(err, cmdLog, "Failed to create xz compressed tarball") {
		return
	}

	sharder.log.Info("Uploading repacked results tarball")

	gsPath := fmt.Sprintf("%s/results.%s-%s.%s.tar.xz", sharder.bucketSubdir, LTMUserName, sharder.testID, sharder.kernelVersion)
	err = sharder.gce.UploadFile(sharder.aggFile+".tar.xz", gsPath)
	check.Panic(err, sharder.log, "Failed to upload results tarball")

	if _, err := gcp.GceConfig.Get("GCE_UPLOAD_SUMMARY"); err == nil {
		gsPath = fmt.Sprintf("%s/summary.%s-%s.%s.txt", sharder.bucketSubdir, LTMUserName, sharder.testID, sharder.kernelVersion)
		err = sharder.gce.UploadFile(sharder.aggDir+"summary", gsPath)
		check.Panic(err, sharder.log, "Failed to upload results summary")
	}
}

// clean removes local result and log files
func (sharder *ShardScheduler) clean() {
	sharder.log.Info("Cleaning up sharder resources")

	if strings.HasSuffix(sharder.gsKernel, "-onerun") {
		sharder.log.WithField("gsKernel", sharder.gsKernel).Info("Delete onerun kernel image")
		sharder.gce.DeleteFiles(sharder.gsKernel)
	}

	sharder.log.Info("Remove local aggregate results")
	os.RemoveAll(sharder.aggDir)
	sharder.gce.Close()
	logging.CloseLog(sharder.log)
}
