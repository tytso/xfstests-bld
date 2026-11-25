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
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"thunk.org/gce-server/util/check"
	"thunk.org/gce-server/util/email"
	"thunk.org/gce-server/util/gcp"
	"thunk.org/gce-server/util/logging"
	"thunk.org/gce-server/util/mymath"
	"thunk.org/gce-server/util/parser"
	"thunk.org/gce-server/util/server"

	"github.com/sirupsen/logrus"
)

const genResultsSummaryPath = "/usr/local/bin/gen_results_summary"
const defaultMonitorTimeout = 1 * time.Hour
const unknownKernelVersion = "unknown_kernel_version"

// ShardScheduler schedules tests and aggregates reports.
type ShardScheduler struct {
	mu        sync.RWMutex
	testID    string
	projID    string
	imgProjID string
	origCmd   string

	zone               string
	region             string
	gsBucket           string
	bucketSubdir       string
	gsKernel           string
	kernelVersion      string
	kernelArch         string
	arch               string
	reportReceiver     string
	reportFailReceiver string
	junitReceiver      string
	maxShards          int
	keepDeadVM         bool
	monitorTimeout     time.Duration

	reportKCS   bool
	testRequest server.TaskRequest
	testResult  server.ResultType
	failed      bool

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

// sharderMap indexes sharders by testID.
var (
	sharderMap      = make(map[string]*ShardScheduler)
	pendingSharders = make(map[string]bool)
	sharderLock     sync.Mutex
)

func reserveSharderID(testID string) bool {
	sharderLock.Lock()
	defer sharderLock.Unlock()
	if _, ok := sharderMap[testID]; ok || pendingSharders[testID] {
		return false
	}
	pendingSharders[testID] = true
	return true
}

// NewShardScheduler constructs a new sharder from a test request.
// All dir strings have a trailing / for consistency purpose,
// except for bucketSubdir.
func NewShardScheduler(c server.TaskRequest, testID string) (*ShardScheduler, error) {
	if err := check.ValidateTestID(testID); err != nil {
		return nil, fmt.Errorf("invalid testID: %w", err)
	}
	if c.Options != nil {
		if c.Options.BucketSubdir != "" {
			if err := check.ValidateBucketSubdir(c.Options.BucketSubdir); err != nil {
				return nil, fmt.Errorf("invalid bucket_subdir: %w", err)
			}
		}
		if c.Options.GsKernel != "" {
			if err := check.ValidateGSURI(c.Options.GsKernel); err != nil {
				return nil, fmt.Errorf("invalid gs_kernel: %w", err)
			}
		}
		if c.Options.Arch != "" {
			if err := check.ValidateArch(c.Options.Arch); err != nil {
				return nil, fmt.Errorf("invalid arch: %w", err)
			}
		}
		if c.Options.MonitorTimeout != "" {
			if _, err := check.ValidateMonitorTimeout(c.Options.MonitorTimeout); err != nil {
				return nil, fmt.Errorf("invalid monitor_timeout: %w", err)
			}
		}
		for _, addr := range []string{c.Options.ReportEmail, c.Options.ReportFailEmail, c.Options.JunitEmail} {
			if addr != "" {
				if err := check.ValidateEmailList(addr); err != nil {
					return nil, fmt.Errorf("invalid email option: %w", err)
				}
			}
		}
	}
	if !reserveSharderID(testID) {
		return nil, fmt.Errorf("testID %s is already running", testID)
	}

	var log *logrus.Entry
	var gceSvc *gcp.Service
	registered := false
	defer func() {
		if !registered {
			sharderLock.Lock()
			delete(pendingSharders, testID)
			sharderLock.Unlock()
			if gceSvc != nil {
				gceSvc.Close()
			}
			if log != nil {
				logging.CloseLog(log)
			}
		}
	}()

	logDir := logging.LTMLogDir + testID + "/"
	err := check.CreateDir(logDir)
	if err != nil {
		return nil, err
	}

	logFile := logDir + "run.log"
	log = logging.InitLogger(logFile)

	origCmd, err := parser.DecodeCmd(c.CmdLine)
	check.Panic(err, log, "Failed to decode cmdline")

	// assume a zone looks like us-central1-f and a region looks like us-central1
	// syntax might change in the future so should add support to query for it
	zone, err := gcp.GceConfig().Get("GCE_ZONE")
	check.Panic(err, log, "Failed to get zone config")
	region := zone[:len(zone)-2]

	projID, err := gcp.GceConfig().Get("GCE_PROJECT")
	check.Panic(err, log, "Failed to get project config")

	imgProjID, err := gcp.GceConfig().Get("GCE_IMAGE_PROJECT")
	check.Panic(err, log, "Failed to get image project")

	gsBucket, err := gcp.GceConfig().Get("GS_BUCKET")
	check.Panic(err, log, "Failed to get gs bucket config")

	bucketSubdir, _ := gcp.GceConfig().Get("BUCKET_SUBDIR")

	log.Info("Initiating test sharder")
	sharder := ShardScheduler{
		testID:    testID,
		projID:    projID,
		imgProjID: imgProjID,
		origCmd:   origCmd,

		zone:               zone,
		region:             region,
		gsBucket:           gsBucket,
		bucketSubdir:       bucketSubdir,
		gsKernel:           c.Options.GsKernel,
		kernelVersion:      unknownKernelVersion,
		kernelArch:         "",
		arch:               c.Options.Arch,
		reportReceiver:     c.Options.ReportEmail,
		reportFailReceiver: c.Options.ReportFailEmail,
		junitReceiver:      c.Options.JunitEmail,
		maxShards:      0,
		keepDeadVM:     false,
		monitorTimeout: defaultMonitorTimeout,

		reportKCS:   false,
		testRequest: c,
		testResult:  server.DefaultResult,
		failed:      false,

		log:     log,
		logDir:  logDir,
		logFile: logFile,
		aggDir:  fmt.Sprintf("%sresults-%s-%s/", logDir, server.LTMUserName, testID),
		aggFile: fmt.Sprintf("%sresults.%s-%s", logDir, server.LTMUserName, testID),
	}

	if _, err := gcp.GceConfig().Get("GCE_LTM_KEEP_DEAD_VM"); err == nil {
		sharder.keepDeadVM = true
	}
	if c.Options.BucketSubdir != "" {
		sharder.bucketSubdir = c.Options.BucketSubdir
	}
	if sharder.bucketSubdir == "" {
		sharder.bucketSubdir = "results"
	}
	err = check.ValidateBucketSubdir(sharder.bucketSubdir)
	check.Panic(err, log, "Invalid bucket_subdir")
	if sharder.reportFailReceiver == "" {
		sharder.reportFailReceiver = sharder.reportReceiver
	}
	if c.Options.MonitorTimeout != "" {
		sharder.monitorTimeout, err = check.ValidateMonitorTimeout(c.Options.MonitorTimeout)
		if err != nil {
			sharder.monitorTimeout = defaultMonitorTimeout
			sharder.log.WithField("MonitorTimeout", c.Options.MonitorTimeout).Error("Unable to parse --monitor-timeout option, using default value")
		} else {
			sharder.log.WithField("MonitorTimeout", sharder.monitorTimeout).Info("Parsed monitor timeout argument")
		}
	}

	sharder.validArgs, sharder.configs, err = getConfigs(sharder.origCmd)
	check.Panic(err, log, "Failed to parse config from origCmd")

	sharder.getKernelInfo()

	sharder.gce, err = gcp.NewService(sharder.gsBucket)
	check.Panic(err, log, "Failed to connect to GCE service")
	gceSvc = sharder.gce

	regionShard := !c.Options.NoRegionShard
	// This is a hack because RegionSharding doesn't know how to
	// exclude zones that don't have arm64 machine types.  More
	// generally, if the user has specified a specific machtype,
	// region sharding doesn't handle that case as well either.
	if c.Options.Arch == "arm64" || c.Options.Arch == "aarch64" {
		regionShard = false
	}
	if regionShard {
		sharder.initRegionSharding()
	} else {
		sharder.initLocalSharding()
	}

	if c.ExtraOptions != nil && c.ExtraOptions.Requester == server.KCSBisectStep {
		sharder.reportKCS = true
	}

	sharderLock.Lock()
	delete(pendingSharders, testID)
	sharderMap[testID] = &sharder
	registered = true
	sharderLock.Unlock()

	return &sharder, nil
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
		shardID := string(rune(i)/26+'a') + string(rune(i)%26+'a')
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

	var usedZones, avoidZones []string
	content, err := os.ReadFile("/usr/local/lib/zone-avoid-list")
	if err == nil {
		avoidZones = strings.Split(string(content), "\n")
	}

	for _, quota := range quotas {
		if !slices.Contains(avoidZones, quota.Zone) &&
			strings.HasPrefix(quota.Zone, continent) {
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
		shardID := string(rune(i)/26+'a') + string(rune(i)%26+'a')
		shard := NewShardWorker(sharder, shardID, config, usedZones[i])
		allShards = append(allShards, shard)
	}

	sharder.shards = allShards
}

// Get information about the kernel so that each gce-xfstests invocation
// doesn't have to replicate this work.
func (sharder *ShardScheduler) getKernelInfo() {
	sharder.kernelArch = ""
	if sharder.gsKernel == "" {
		return
	}
	if err := check.ValidateGSURI(sharder.gsKernel); err != nil {
		sharder.log.WithError(err).Error("Refusing to query invalid gsKernel URI")
		return
	}
	cmd := exec.Command("gce-xfstests", "get-kernel-info", sharder.gsKernel)
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if !check.NoError(err, sharder.log, "Failed to get kernel information") {
		sharder.log.Errorf("Failure text: %s",
			strings.TrimSuffix(stderr.String(), "\n"))
		return
	}
	for _, v := range strings.Split(stdout.String(), "\n") {
		arg := strings.Split(v, "=")
		if len(arg) < 2 {
			continue
		}
		if arg[0] == "KERNEL_ARCH" {
			sharder.kernelArch = strings.TrimSuffix(arg[1], "\n")
		} else if arg[0] == "KERNEL_VERSION" {
			ver := strings.TrimSuffix(arg[1], "\n")
			if err := check.ValidateTestID(ver); err != nil {
				sharder.log.WithField("kernelVersion", ver).WithError(err).Warn("Ignoring invalid kernel version from get-kernel-info")
			} else {
				sharder.setKernelVersion(ver)
			}
		}
	}
}

// getConfigs calls a parser to extract the valid args and configs from
// the raw cmdline.
func getConfigs(origCmd string) ([]string, []string, error) {
	validArgs, configs, err := parser.Cmd(origCmd)
	if err != nil {
		return []string{}, []string{}, err
	}
	configStrings := []string{}
	for fs := range configs {
		for _, cfg := range configs[fs] {
			configStrings = append(configStrings, fs+"/"+cfg)
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
func (sharder *ShardScheduler) Run() {
	sharder.log.Debug("Starting sharder")
	var wg sync.WaitGroup

	defer logging.CloseLog(sharder.log)

	defer email.ReportFailureFunc(sharder.log, sharder.logFile, sharder.reportFailReceiver, func() string {
		return fmt.Sprintf("xfstests failure %s-%s %s", server.LTMUserName, sharder.testID, sharder.getKernelVersion())
	})

	defer sharder.clean()

	if sharder.reportKCS {
		defer sharder.sendKCSReport()
	}

	defer sharder.sendWatcherResult()

	for _, shard := range sharder.shards {
		wg.Add(1)
		go shard.Run(&wg)
	}
	wg.Wait()

	sharder.log.Debug("All shards finished")
	sharder.finish()
}

func (sharder *ShardScheduler) getKernelVersion() string {
	sharder.mu.RLock()
	defer sharder.mu.RUnlock()
	return sharder.kernelVersion
}

func (sharder *ShardScheduler) setKernelVersion(v string) {
	sharder.mu.Lock()
	sharder.kernelVersion = v
	sharder.mu.Unlock()
}

func (sharder *ShardScheduler) setKernelVersionIfUnknown(v string) bool {
	if err := check.ValidateTestID(v); err != nil {
		return false
	}
	sharder.mu.Lock()
	defer sharder.mu.Unlock()
	if sharder.kernelVersion != "" && sharder.kernelVersion != unknownKernelVersion {
		return false
	}
	sharder.kernelVersion = v
	return true
}

func (sharder *ShardScheduler) getResult() server.ResultType {
	sharder.mu.RLock()
	defer sharder.mu.RUnlock()
	return sharder.testResult
}

func (sharder *ShardScheduler) setResult(r server.ResultType) {
	sharder.mu.Lock()
	sharder.testResult = r
	sharder.mu.Unlock()
}

// Info returns structured sharder information.
func (sharder *ShardScheduler) Info() server.SharderInfo {
	sharder.mu.RLock()
	kernelVersion := sharder.kernelVersion
	result := sharder.testResult
	sharder.mu.RUnlock()

	info := server.SharderInfo{
		ID:            sharder.testID,
		Command:       sharder.origCmd,
		KernelVersion: kernelVersion,
		KernelArch:    sharder.kernelArch,
		NumShards:     len(sharder.shards),
		Result:        result.String(),
	}

	for _, shard := range sharder.shards {
		info.ShardInfo = append(info.ShardInfo, shard.Info())
	}

	return info
}

// finish aggregates results and upload a tarball to gs bucket.
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
		shardHasResults := false

		if check.DirExists(shard.unpackedResultsDir) {
			err := os.RemoveAll(sharder.aggDir + shard.shardID)
			check.Panic(err, log, "Failed to remove dir")

			err = os.Rename(shard.unpackedResultsDir, sharder.aggDir+shard.shardID)
			check.Panic(err, log, "Failed to move dir")

			shardHasResults = true
			hasResults = true
		}

		if check.FileExists(shard.serialOutputPath) {
			err := os.RemoveAll(sharder.aggDir + shard.shardID + ".serial")
			check.Panic(err, log, "Failed to remove dir")

			err = os.Rename(shard.serialOutputPath, sharder.aggDir+shard.shardID+".serial")
			check.Panic(err, log, "Failed to move dir")

			shardHasResults = true
			hasResults = true
		}

		if !shardHasResults {
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
		if v := sharder.getKernelVersion(); v != "" && v != unknownKernelVersion {
			break
		}
		kernelVersionFile := fmt.Sprintf("%s%s/kernel_version", sharder.aggDir, shard.shardID)
		if check.FileExists(kernelVersionFile) {
			content, err := check.ReadLines(kernelVersionFile)
			if !check.NoError(err, sharder.log, "Failed to read file") || len(content) == 0 {
				continue
			}
			if !sharder.setKernelVersionIfUnknown(content[0]) {
				sharder.log.WithField("kernelVersion", content[0]).Warn("Ignoring invalid kernel_version from shard")
				continue
			}
		}
	}
}

// concatResults aggregate all shard files of a given file type by producing
// a concatenated file at the top level of the aggregate results directory.
// If test result is not found for any shard, testResult is set to unknown.
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

	fmt.Fprintf(file, "TESTRUNID: %s-%s\n", server.LTMUserName, sharder.testID)
	fmt.Fprintf(file, "CMDLINE: %s\n", sharder.origCmd)

}

/*
genResultsSummary generate test result summary and determine the test result status.

It calls a python script to generate the summary on junit xml test results and parses
the summary file to check for any test failures.
It also checks testResult from each shard. The final testResult is:

Fail	a crash, hang or failed test happens;
Error	any shard has error, test result is not found or other unexpected errors;
Pass	nothing above happens and no test failure found.

If any shard has non-default testResult, append the sharder info to the result file.
*/
func (sharder *ShardScheduler) genResultsSummary() {
	sharder.log.Info("Creating LTM test result summary")
	cmd := exec.Command(genResultsSummaryPath, sharder.aggDir, "--output_file", sharder.aggDir+"report", "--merge_file", sharder.aggDir+"results.xml", "--check_failure")


	cmdLog := sharder.log.WithField("cmd", cmd.String())
	w := cmdLog.Writer()
	defer w.Close()
	err := check.LimitedRun(cmd, check.RootDir, check.EmptyEnv, w, w)
	if !check.NoError(err, cmdLog, "Failed to run python script gen_results_summary") {
		sharder.failed = true
	}

	_, err = os.Stat(sharder.aggDir+"report.failed")
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		sharder.failed = true
	}

	var testFailure, testError, reportInfo bool

	for _, shard := range sharder.shards {
		switch shard.getResult() {
		case server.DefaultResult:
			// do nothing, won't fallthrough like in C
		case server.Error:
			testError = true
			reportInfo = true
			sharder.failed = true
		case server.Pass:
			fallthrough
		case server.Fail:
			sharder.log.Error("shard may not produce a pass or fail result")
		default:
			testFailure = true
			reportInfo = true
			sharder.failed = true
		}
	}

	content, err := ioutil.ReadFile(sharder.aggDir + "report")
	if check.NoError(err, sharder.log, "Failed to read the report file") {
		if !strings.Contains(string(content), "0 failures") {
			testFailure = true
		}
	}

	if testFailure {
		sharder.setResult(server.Fail)
	} else if testError {
		sharder.setResult(server.Error)
	} else {
		sharder.setResult(server.Pass)
	}

	if reportInfo {
		file, err := os.OpenFile(sharder.aggDir+"report", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if !check.NoError(err, sharder.log, "failed to open report file") {
			return
		}
		defer file.Close()

		fmt.Fprintf(file, "\nSome shard finished with problems:\n%s", sharder.Info().String())
	}
}

// emailReport sends the email.
func (sharder *ShardScheduler) emailReport() {
	kernelVersion := sharder.getKernelVersion()
	if sharder.reportReceiver == "" && sharder.reportFailReceiver == "" {
		sharder.log.Info("Skipping e-mail report")
	} else {
		subject := fmt.Sprintf("xfstests results %s-%s %s", server.LTMUserName, sharder.testID, kernelVersion)

		b, err := ioutil.ReadFile(sharder.aggDir + "report")
		content := string(b)
		if !check.NoError(err, sharder.log, "Failed to read the report file") {
			content = "Unable to generate test summary report"
			sharder.failed = true
		}

		emailReceiver := sharder.reportReceiver
		if sharder.failed {
			emailReceiver = sharder.reportFailReceiver
			sharder.log.Info("Sharder found failure")
		}

		if emailReceiver != "" {
			sharder.log.Info("Sending e-mail report to " + emailReceiver)
			err = email.Send(subject, content, emailReceiver)
			check.Panic(err, sharder.log, "Failed to send the report email")
		} else {
			sharder.log.Info("Skipping e-mail report")
		}
	}

	if sharder.junitReceiver == "" {
		sharder.log.Info("Skipping junit file e-mail")
	} else {
		sharder.log.Info("Sending junit file e-mail")
		subject := fmt.Sprintf("xfstests junit results %s-%s %s", server.LTMUserName, sharder.testID, kernelVersion)

		b, err := ioutil.ReadFile(sharder.aggDir + "results.xml")
		content := string(b)
		if !check.NoError(err, sharder.log, "Failed to read the results xml file") {
			content = "Unable to generate combined junit file"
		}

		err = email.Send(subject, content, sharder.junitReceiver)
		check.Panic(err, sharder.log, "Failed to send the junit email")
	}
}

func splitParentTestID(testID string) (parent string, suffix string, ok bool) {
	idx := strings.LastIndex(testID, "-")
	if idx <= 0 || idx == len(testID)-1 {
		return "", "", false
	}
	return testID[:idx], testID[idx+1:], true
}

func (sharder *ShardScheduler) bisectReportRequest() (server.TaskRequest, error) {
	parentID, _, ok := splitParentTestID(sharder.testID)
	if !ok {
		return server.TaskRequest{}, fmt.Errorf("cannot extract parent bisect testID from %q", sharder.testID)
	}
	req := sharder.testRequest
	var extraCopy server.InternalOptions
	if req.ExtraOptions != nil {
		extraCopy = *req.ExtraOptions
	}
	extraCopy.TestID = parentID
	extraCopy.TestResult = sharder.getResult()
	extraCopy.Requester = server.LTMBisectStep
	req.ExtraOptions = &extraCopy
	return req, nil
}

func (sharder *ShardScheduler) sendKCSReport() {
	req, err := sharder.bisectReportRequest()
	check.Panic(err, sharder.log, "Failed to build bisect step report")

	server.SendInternalRequest(req, sharder.log, true)
}

func (sharder *ShardScheduler) sendWatcherResult() {
	UpdateWatcherTest(sharder.testID, sharder.getResult())
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
	kernelVersion := sharder.getKernelVersion()
	if err := check.ValidateTestID(kernelVersion); err != nil {
		sharder.log.WithField("kernelVersion", kernelVersion).WithError(err).Warn("Invalid kernelVersion when packing results, using default")
		kernelVersion = unknownKernelVersion
	}

	gsPath := fmt.Sprintf("%s/results.%s-%s.%s.tar.xz", sharder.bucketSubdir, server.LTMUserName, sharder.testID, kernelVersion)
	err = sharder.gce.UploadFile(sharder.aggFile+".tar.xz", gsPath)
	check.Panic(err, sharder.log, "Failed to upload results tarball")

	gsPath = fmt.Sprintf("%s/results.%s-%s.%s.xml", sharder.bucketSubdir, server.LTMUserName, sharder.testID, kernelVersion)
	err = sharder.gce.UploadFile(sharder.aggDir+"results.xml", gsPath)
	check.Panic(err, sharder.log, "Failed to upload junit file")

	os.Remove(sharder.aggFile + ".tar.xz")

	if _, err := gcp.GceConfig().Get("GCE_UPLOAD_SUMMARY"); err == nil {
		gsPath = fmt.Sprintf("%s/summary.%s-%s.%s.txt", sharder.bucketSubdir, server.LTMUserName, sharder.testID, kernelVersion)
		err = sharder.gce.UploadFile(sharder.aggDir+"summary", gsPath)
		check.Panic(err, sharder.log, "Failed to upload results summary")
	}
}

func onerunObjectName(gsBucket string, gsKernel string) (string, bool) {
	if gsBucket == "" || !strings.Contains(gsKernel, "-onerun") {
		return "", false
	}
	if err := check.ValidateGSURI(gsKernel); err != nil {
		return "", false
	}
	prefix := "gs://" + gsBucket + "/"
	if !strings.HasPrefix(gsKernel, prefix) {
		return "", false
	}
	obj := strings.TrimPrefix(gsKernel, prefix)
	if obj == "" {
		return "", false
	}
	return obj, true
}

// clean removes local result and log files.
func (sharder *ShardScheduler) clean() {
	sharder.log.Info("Cleaning up sharder resources")

	if objName, ok := onerunObjectName(sharder.gsBucket, sharder.gsKernel); ok && sharder.gce != nil {
		sharder.log.WithField("gsKernel", sharder.gsKernel).Info("Delete onerun kernel image")
		_, err := sharder.gce.DeleteFiles(objName)
		check.NoError(err, sharder.log, "Failed to delete onerun kernel image")
	}
	sharderLock.Lock()
	delete(sharderMap, sharder.testID)
	sharderLock.Unlock()

	sharder.log.Info("Remove local aggregate results")
	os.RemoveAll(sharder.aggDir)
	if sharder.gce != nil {
		sharder.gce.Close()
	}
}

// SharderStatus returns the info for running sharders.
func SharderStatus() []server.SharderInfo {
	sharderLock.Lock()
	sharders := make([]*ShardScheduler, 0, len(sharderMap))
	for _, v := range sharderMap {
		sharders = append(sharders, v)
	}
	sharderLock.Unlock()

	infoList := make([]server.SharderInfo, 0, len(sharders))
	for _, v := range sharders {
		infoList = append(infoList, v.Info())
	}
	sort.Slice(infoList, func(i, j int) bool {
		return infoList[i].ID < infoList[j].ID
	})
	return infoList
}
