/*
ShardSchedular arrages the tests and runs them in multiple shardWorkers.

The sharder parses the command line arguments sent by user, parse it into
machine understandable xfstests configs. Then it queries for GCE quotas and
spawns a suitable number of shards to run the tests. The sharder waits until
all shards finish, fetch the result files and aggregate them. An email is sent
to the user if necessary.

The TestRunManager from previous flask version is integrated into shardSchedular
now to reduce the code base.

*/
package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"gce-server/logging"
	"gce-server/server"
	"gce-server/util"

	"github.com/sirupsen/logrus"
)

const genResultsSummaryPath = "/usr/local/bin/gen_results_summary"

// ShardSchedular schedules tests and aggregates reports.
type ShardSchedular struct {
	testID  string
	projID  string
	origCmd string

	gitRepo  string
	commitID string
	callKCS  bool

	zone           string
	region         string
	gsBucket       string
	bucketSubdir   string
	gsKernel       string
	kernelVersion  string
	reportReceiver string
	maxShards      int
	keepDeadVM     bool

	log     *logrus.Entry
	logDir  string
	logFile string
	aggDir  string
	aggFile string

	validArgs []string
	configs   []string
	gce       *util.GceService
	shards    []*ShardWorker
}

// SharderInfo exports sharder info to be sent back to user.
type SharderInfo struct {
	NumShards int         `json:"num_shards"`
	ShardInfo []ShardInfo `json:"shard_info"`
	ID        string      `json:"id"`
	Msg       string      `json:"message"`
}

// NewShardSchedular constructs a new sharder from user request.
// All dir strings have a trailing / for consistency purpose,
// except for bucketSubdir.
func NewShardSchedular(c server.UserRequest) *ShardSchedular {
	testID := util.GetTimeStamp()
	logDir := logging.LTMLogDir + testID + "/"
	err := util.CreateDir(logDir)
	if err != nil {
		panic(err)
	}

	logFile := logDir + "run.log"
	log := logging.InitLogger(logFile)

	config, err := util.GetConfig(util.GceConfigFile)
	logging.CheckPanic(err, log, "Failed to get config")

	// assume a zone looks like us-central1-f and a region looks like us-central1
	// syntax might change in the future so should add support to query for it
	zone := config.Get("GCE_ZONE")
	region := zone[:len(zone)-2]

	log.Info("Initializing test sharder")
	sharder := ShardSchedular{
		testID:  testID,
		projID:  config.Get("GCE_PROJECT"),
		origCmd: strings.TrimSpace(c.CmdLine),

		gitRepo:  c.Options.GitRepo,
		commitID: c.Options.CommitID,
		callKCS:  false,

		zone:           zone,
		region:         region,
		gsBucket:       config.Get("GS_BUCKET"),
		bucketSubdir:   config.Get("BUCKET_SUBDIR"),
		gsKernel:       "",
		kernelVersion:  "unknown_kernel_version",
		reportReceiver: "",
		maxShards:      0,
		keepDeadVM:     false,

		log:     log,
		logDir:  logDir,
		logFile: logFile,
		aggDir:  fmt.Sprintf("%sresults-%s-%s/", logDir, LTMUserName, testID),
		aggFile: fmt.Sprintf("%sresults.%s-%s", logDir, LTMUserName, testID),
	}

	if config.Get("GCE_LTM_KEEP_DEAD_VM") != "" {
		sharder.keepDeadVM = true
	}
	if c.Options.BucketSubdir != "" {
		sharder.bucketSubdir = c.Options.BucketSubdir
	}
	if sharder.bucketSubdir == "" {
		sharder.bucketSubdir = "results"
	}
	if sharder.gitRepo != "" && sharder.commitID != "" {
		log.Debug("Build is requested, override gsKernel when launching the shards")
		sharder.callKCS = true
		sharder.gsKernel = fmt.Sprintf("gs://%s/kernels/bzImage-%s", sharder.gsBucket, sharder.testID)
	} else {
		sharder.gsKernel = c.Options.GsKernel
	}

	if c.Options.ReportEmail != "" {
		sharder.reportReceiver = c.Options.ReportEmail
	}

	sharder.validArgs, sharder.configs, err = getConfigs(sharder.origCmd)
	logging.CheckPanic(err, log, "Failed to parse config from origCmd")

	sharder.gce, err = util.NewGceService(sharder.gsBucket)
	logging.CheckPanic(err, log, "Failed to connect to GCE service")

	regionShard := !c.Options.NoRegionShard
	if regionShard {
		sharder.initRegionSharding()
	} else {
		sharder.initLocalSharding()
	}

	return &sharder
}

// initLocalSharding creates shards in the same zone the VM runs in.
// The sharder queries for available quotas in the current zone and
// spawns new shards accordingly.
func (sharder *ShardSchedular) initLocalSharding() {
	log := sharder.log.WithField("region", sharder.region)
	log.Info("Initilizing local sharding")
	allShards := []*ShardWorker{}
	quota, err := sharder.gce.GetRegionQuota(sharder.projID, sharder.region)
	logging.CheckPanic(err, log, "Failed to get quota")

	if quota == nil {
		log.Panic("GCE region is out of quota")
	}
	numShards, err := quota.GetMaxShard()
	logging.CheckPanic(err, log, "Failed to get max shard")

	if sharder.maxShards > 0 {
		numShards = util.MaxInt(numShards, sharder.maxShards)
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
func (sharder *ShardSchedular) initRegionSharding() {
	continent := strings.Split(sharder.region, "-")[0]
	log := sharder.log.WithField("continent", continent)
	log.Info("Initilizing region sharding")

	allShards := []*ShardWorker{}
	quotas, err := sharder.gce.GetAllRegionsQuota(sharder.projID)
	logging.CheckPanic(err, log, "Failed to get quota")

	usedZones := []string{}

	for _, quota := range quotas {
		if strings.HasPrefix(quota.Zone, continent) {
			maxShard, err := quota.GetMaxShard()
			logging.CheckPanic(err, log, "Failed to get max shard")

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
				logging.CheckPanic(err, log, "Failed to get max shard")

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
	validArgs, configs, err := util.ParseCmd(origCmd)
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

// StartTests launches all the shards in a separate go routine.
// Wait between starting shards to avoid hitting the api too hard.
// Return a JSONifiable sharder info struct to return to user.
func (sharder *ShardSchedular) StartTests() SharderInfo {
	go sharder.run()

	return sharder.Info()
}

func (sharder *ShardSchedular) run() {
	sharder.log.Debug("Starting sharder")
	var wg sync.WaitGroup

	defer sharder.reportFailure()

	if sharder.callKCS {
		sharder.log.Info("Calling KCS to build kernel")
		RunBuild(sharder.gitRepo, sharder.commitID, sharder.reportReceiver, sharder.testID, sharder.gce, sharder.log)
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

// Info returns structured sharder information to send back to user.
func (sharder *ShardSchedular) Info() SharderInfo {
	info := SharderInfo{
		NumShards: len(sharder.shards),
		ID:        sharder.testID,
	}

	for _, shard := range sharder.shards {
		info.ShardInfo = append(info.ShardInfo, shard.Info())
	}

	if sharder.callKCS {
		info.Msg = "calling KCS to build kernel image"
	}

	return info
}

// aggregate results and upload a tarball to gs bucket.
// panic and send an email to user if no results available.
func (sharder *ShardSchedular) finish() {
	sharder.log.Debug("Finishing sharder")

	sharder.aggResults()
	sharder.createInfo()
	sharder.createRunStats()
	genResultsSummary(sharder.aggDir, sharder.aggDir+"report", sharder.log)
	sharder.emailReport()
	sharder.packResults()

	sharder.cleanup()
}

// aggResults looks for results file from each shard and aggregates them.
func (sharder *ShardSchedular) aggResults() {
	err := util.CreateDir(sharder.aggDir)
	logging.CheckPanic(err, sharder.log, "Failed to create dir")

	hasResults := false
	for _, shard := range sharder.shards {
		log := sharder.log.WithFields(logrus.Fields{
			"shardID":            shard.shardID,
			"unpackedResultsDir": shard.unpackedResultsDir,
		})
		log.Debug("Moving shard result files into aggregate folder")

		if util.DirExists(shard.unpackedResultsDir) {
			err := util.RemoveDir(sharder.aggDir + shard.shardID)
			logging.CheckPanic(err, log, "Failed to remove dir")

			err = os.Rename(shard.unpackedResultsDir, sharder.aggDir+shard.shardID)
			logging.CheckPanic(err, log, "Failed to move dir")

			hasResults = true
		} else if util.FileExists(shard.serialOutputPath) {
			err := util.RemoveDir(sharder.aggDir + shard.shardID + ".serial")
			logging.CheckPanic(err, log, "Failed to remove dir")

			err = os.Rename(shard.serialOutputPath, sharder.aggDir+shard.shardID+".serial")
			logging.CheckPanic(err, log, "Failed to move dir")

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
		if util.FileExists(kernelVersionFile) {
			content, err := util.ReadLines(kernelVersionFile)
			if !logging.CheckNoError(err, sharder.log, "Failed to read file") {
				continue
			}
			sharder.kernelVersion = content[0]
		}
	}
}

// concatResults aggregate all shard files of a given file type by producing
// a concatenated file at the top level of the aggregate results directory.
func (sharder *ShardSchedular) concatResults(filename string) {
	log := sharder.log.WithField("resultFile", filename)
	log.Info("Cancatenating shard result file")

	file, err := os.Create(sharder.aggDir + filename)
	logging.CheckPanic(err, log, "Failed to create file")

	defer file.Close()

	fmt.Fprintf(file, "LTM aggregate file for %s\n", filename)
	fmt.Fprintf(file, "Test run ID %s\n", sharder.testID)
	fmt.Fprintf(file, "Aggregate results from %d shards\n", len(sharder.shards))

	for _, shard := range sharder.shards {
		shardLog := log.WithField("shardID", shard.shardID)
		fmt.Fprintf(file, "\n============SHARD %s============\n", shard.shardID)
		fmt.Fprintf(file, "============CONFIG: %s\n\n", shard.config)
		shardFile := fmt.Sprintf("%s%s/%s", sharder.aggDir, shard.shardID, filename)
		if util.FileExists(shardFile) {
			sourceFile, err := os.Open(shardFile)
			if logging.CheckNoError(err, shardLog, "Failed to open file") {
				_, err = io.Copy(file, sourceFile)
				logging.CheckNoError(err, shardLog, "Failed to copy file")

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
func (sharder *ShardSchedular) createInfo() {
	sharder.log.Info("Creating LTM info")
	ltmLogDir := sharder.aggDir + "ltm_logs/"
	err := util.CreateDir(ltmLogDir)
	if !logging.CheckNoError(err, sharder.log, "Failed to create dir") {
		return
	}

	file, err := os.Create(sharder.aggDir + "ltm-info")
	if !logging.CheckNoError(err, sharder.log, "Failed to create file") {
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
		//TODO: fix the tricky log dir moving around stuffs
	}

	sharder.log.Info("Finished creating ltm-info")
}

func (sharder *ShardSchedular) createRunStats() {
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

// genResultsSummary call a python script to generate the summary on junit xml test results.
func genResultsSummary(resultsDir string, outputFile string, log *logrus.Entry) {
	cmd := exec.Command(genResultsSummaryPath, resultsDir, "--output_file", outputFile)
	cmdLog := log.WithField("cmd", cmd.String())
	w := cmdLog.Writer()
	defer w.Close()
	err := util.CheckRun(cmd, util.RootDir, util.EmptyEnv, w, w)
	logging.CheckNoError(err, cmdLog, "Failed to run python script gen_results_summary")
}

func (sharder *ShardSchedular) emailReport() {
	sharder.log.Info("Sending email report")
	subject := fmt.Sprintf("xfstests results %s-%s %s", LTMUserName, sharder.testID, sharder.kernelVersion)

	content, err := ioutil.ReadFile(sharder.aggDir + "report")
	logging.CheckPanic(err, sharder.log, "Failed to read the report file, cannot send email")

	err = util.SendEmail(subject, string(content), sharder.reportReceiver)
	logging.CheckPanic(err, sharder.log, "Failed to send the email")
}

// packResults packs the aggregared files after copying the sharder's log file into it.
func (sharder *ShardSchedular) packResults() {
	sharder.log.Info("Packing aggregated files")
	sharder.log.Info("Copying sharder log file")

	logging.Sync(sharder.log)
	aggLogFile := sharder.aggDir + "ltm_logs/run.log"
	err := util.CopyFile(aggLogFile, sharder.logFile)
	if err != nil {
		logging.CheckPanic(err, sharder.log, "Failed to copy sharder log file")
	}

	cmd1 := exec.Command("tar", "-cf", sharder.aggFile+".tar", "-C", sharder.aggDir, ".")
	cmdLog1 := sharder.log.WithField("cmd", cmd1.Args)
	w1 := cmdLog1.Writer()
	defer w1.Close()
	err = util.CheckRun(cmd1, util.RootDir, util.EmptyEnv, w1, w1)
	if !logging.CheckNoError(err, cmdLog1, "Failed to create tarball") {
		return
	}

	cmd2 := exec.Command("xz", "-6ef", sharder.aggFile+".tar")
	cmdLog2 := sharder.log.WithField("cmd2", cmd2.Args)
	w2 := cmdLog1.Writer()
	defer w2.Close()
	err = util.CheckRun(cmd2, util.RootDir, util.EmptyEnv, w2, w2)
	if !logging.CheckNoError(err, cmdLog2, "Failed to create xz compressed tarball") {
		return
	}

	sharder.log.Info("Uploading repacked results tarball")

	gsPath := fmt.Sprintf("%s/results.%s-%s.%s.tar.xz", sharder.bucketSubdir, LTMUserName, sharder.testID, sharder.kernelVersion)
	err = sharder.gce.UploadFile(sharder.aggFile+".tar.xz", gsPath)
	logging.CheckPanic(err, sharder.log, "Failed to upload results tarball")

	config, err := util.GetConfig(util.GceConfigFile)
	logging.CheckPanic(err, sharder.log, "Failed to get gce config")

	if config.Get("GCE_UPLOAD_SUMMARY") != "" {
		gsPath = fmt.Sprintf("%s/summary.%s-%s.%s.txt", sharder.bucketSubdir, LTMUserName, sharder.testID, sharder.kernelVersion)
		err = sharder.gce.UploadFile(sharder.aggDir+"summary", gsPath)
		logging.CheckPanic(err, sharder.log, "Failed to upload results summary")
	}
}

// cleanup removes local result and log files
// Closes sharder logger handler (if any) here
func (sharder *ShardSchedular) cleanup() {
	sharder.log.Info("Cleaning up sharder resources")

	if strings.HasSuffix(sharder.gsKernel, "-onerun") {
		sharder.log.WithField("gsKernel", sharder.gsKernel).Info("Delete onerun kernel image")
		sharder.gce.DeleteFiles(sharder.gsKernel)
	}

	sharder.log.Info("Remove local aggregate results")
	util.RemoveDir(sharder.aggDir)

	sharder.log.Info("Existing logging, sharder finished")
	logging.CloseLog(sharder.log)
}

func (sharder *ShardSchedular) reportFailure() {
	if r := recover(); r != nil {
		sharder.log.WithField("panic", r).Warn("Sharder failed, sending failure report")

		subject := fmt.Sprintf("xfstests failure %s-%s %s", LTMUserName, sharder.testID, sharder.kernelVersion)

		msg := "unknown panic"
		switch s := r.(type) {
		case string:
			msg = s
		case error:
			msg = s.Error()
		case *logrus.Entry:
			msg = s.Message
		}

		logging.Sync(sharder.log)

		if util.FileExists(sharder.logFile) {
			sharder.log.WithField("logfile", sharder.logFile).Debug("Attaching log file to email")

			content, err := ioutil.ReadFile(sharder.logFile)
			if logging.CheckNoError(err, sharder.log, "Failed to read sharder log file") {
				msg = msg + "\n\n" + string(content)
			}
		}

		err := util.SendEmail(subject, msg, sharder.reportReceiver)
		logging.CheckNoError(err, sharder.log, "Failed to send the email")
	}
}
