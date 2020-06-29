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
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"example.com/gce-server/util"
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

	logDir  string
	logFile string
	aggDir  string
	aggFile string

	validArgs []string
	configs   []string
	gce       util.GceService
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
// All directory strings have a trailing / for consistency purpose,
// except for bucketSubdir.
func NewShardSchedular(c util.UserRequest) *ShardSchedular {
	testID := util.GetTimeStamp()
	logDir := util.TestLogDir + testID + "/"
	util.CreateDir(logDir)

	config := util.GetConfig()
	// assume a zone looks like us-central1-f and a region looks like us-central1
	// syntax might change in the future so should add support to query for it
	zone := config.Get("GCE_ZONE")
	region := zone[:len(zone)-2]

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

		logDir:  logDir,
		logFile: logDir + "run.log",
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
		// overwrite gsKernel when calling kcs to build the kernel
		sharder.callKCS = true
		sharder.gsKernel = fmt.Sprintf("gs://%s/kernels/bzImage-%s", sharder.gsBucket, sharder.testID)
	} else {
		sharder.gsKernel = c.Options.GsKernel
	}

	if c.Options.ReportEmail != "" {
		sharder.reportReceiver = c.Options.ReportEmail
	}

	sharder.validArgs, sharder.configs = getConfigs(sharder.origCmd)
	sharder.gce = util.NewGceService(sharder.gsBucket)
	sharder.setupLogging()

	regionShard := !c.Options.NoRegionShard
	if regionShard {
		sharder.initRegionSharding()
	} else {
		sharder.initLocalSharding()
	}

	return &sharder
}

func (sharder *ShardSchedular) setupLogging() {
}

// initLocalSharding creates shards in the same zone the VM runs in.
// The sharder queries for available quotas in the current zone and
// spawns new shards accordingly.
func (sharder *ShardSchedular) initLocalSharding() {
	allShards := []*ShardWorker{}
	quota := sharder.gce.GetRegionQuota(sharder.projID, sharder.region)
	if quota == nil {
		log.Fatalf("GCE region %s is out of quota\n", sharder.region)
	}
	numShards := quota.GetMaxShard()
	if sharder.maxShards > 0 {
		numShards = util.MaxInt(numShards, sharder.maxShards)
	}
	configs := splitConfigs(numShards, sharder.configs)

	for i, config := range configs {
		shardID := string(i/26+int('a')) + string(i%26+int('a'))
		shard := NewShardWorker(sharder, shardID, config, sharder.zone, sharder.validArgs)
		allShards = append(allShards, shard)
	}

	sharder.shards = allShards
}

// initRegionSharding creates shards among all zones with available quotas.
func (sharder *ShardSchedular) initRegionSharding() {
	allShards := []*ShardWorker{}
	quotas := sharder.gce.GetAllRegionsQuota(sharder.projID)
	usedZones := []string{}
	continent := strings.Split(sharder.region, "-")[0]

	for _, quota := range quotas {
		if strings.HasPrefix(quota.Zone, continent) {
			for i := 0; i < quota.GetMaxShard(); i++ {
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
				for i := 0; i < quota.GetMaxShard(); i++ {
					usedZones = append(usedZones, quota.Zone)
				}
			}
			if len(usedZones) >= len(sharder.configs) {
				break
			}
		}
	}
	configs := splitConfigs(len(usedZones), sharder.configs)

	for i, config := range configs {
		shardID := string(i/26+int('a')) + string(i%26+int('a'))
		shard := NewShardWorker(sharder, shardID, config, usedZones[i], sharder.validArgs)
		allShards = append(allShards, shard)
	}

	sharder.shards = allShards
}

// getConfigs calls a parser to extract the valid args and configs from
// the raw cmdline.
func getConfigs(origCmd string) ([]string, []string) {
	validArgs, configs := util.ParseCmd(origCmd)
	configStrings := []string{}
	for fs := range configs {
		for _, cfg := range configs[fs] {
			if cfg != "dax" {
				configStrings = append(configStrings, fs+"/"+cfg)
			}
		}
	}
	return validArgs, configStrings
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
func (sharder *ShardSchedular) StartTests() SharderInfo {
	go sharder.run()

	return sharder.Info()
}

func (sharder *ShardSchedular) run() {
	var wg sync.WaitGroup

	if sharder.callKCS {
		go sharder.runBuild(&wg)
	}

	for _, shard := range sharder.shards {
		wg.Add(1)
		log.Printf("run shard %+v\n", shard)
		go shard.Run(&wg)
		time.Sleep(500 * time.Millisecond)
	}
	wg.Wait()

	log.Printf("all shards finished")
	sharder.finish()
}

func (sharder *ShardSchedular) runBuild(wg *sync.WaitGroup) {
	log.Printf("launching kcs server")
	cmd := exec.Command("gce-xfstests", "launch-kcs")
	status := util.CheckRun(cmd, util.RootDir, util.EmptyEnv, os.Stdout, os.Stderr)
	if !status {
		log.Fatalf("KCS failed to start")
	}

	args1 := util.UserOptions{
		GitRepo:  sharder.gitRepo,
		CommitID: sharder.commitID,
	}
	args2 := util.LTMOptions{
		TestID: sharder.testID,
	}
	request := util.UserRequest{
		Options:      &args1,
		ExtraOptions: &args2,
	}
	log.Printf("%+v", request)
}

// Info returns structured sharder information to send back to user.
func (sharder *ShardSchedular) Info() SharderInfo {
	if sharder.callKCS {
		info := SharderInfo{
			ID:  sharder.testID,
			Msg: "calling KCS to build kernel image",
		}
		return info
	}

	info := SharderInfo{
		NumShards: len(sharder.shards),
		ID:        sharder.testID,
	}

	for i, shard := range sharder.shards {
		info.ShardInfo = append(info.ShardInfo, shard.Info(i))
	}

	return info
}

// aggregate results and upload a tarball to gs bucket.
func (sharder *ShardSchedular) finish() {
	log.Printf("Finishing sharder")

	if sharder.aggResults() {
		sharder.createInfo()
		sharder.createRunStats()
		genResultsSummary(sharder.aggDir, sharder.aggDir+"report")
		sharder.emailReport()
		sharder.packResults()
	} else {
		log.Printf("No result files uploaded")
	}

	sharder.cleanup()

}

// aggResults looks for results file from each shard and aggregates them.
func (sharder *ShardSchedular) aggResults() bool {
	util.CreateDir(sharder.aggDir)

	hasResults := false
	for _, shard := range sharder.shards {
		if util.DirExists(shard.unpackedResultsDir) {
			util.RemoveDir(sharder.aggDir + shard.shardID)
			err := os.Rename(shard.unpackedResultsDir, sharder.aggDir+shard.shardID)
			util.Check(err)
			hasResults = true
		} else if util.FileExists(shard.serialOutputPath) {
			util.RemoveDir(sharder.aggDir + shard.shardID + ".serial")
			err := os.Rename(shard.serialOutputPath, sharder.aggDir+shard.shardID+".serial")
			util.Check(err)
			hasResults = true
		} else {
			log.Printf("Shard %s has no results available", shard.shardID)
		}
	}
	if !hasResults {
		log.Printf("No results available for any of the shards")
		return false
	}

	for _, config := range []string{"runtests.log", "cmdline", "summary", "failures", "run-stats",
		"testrunid", "kernel_version"} {
		sharder.concatResults(config)
	}

	for _, shard := range sharder.shards {
		kernelVersionFile := fmt.Sprintf("%s%s/kernel_version", sharder.aggDir, shard.shardID)
		if util.FileExists(kernelVersionFile) {
			content, err := util.ReadLines(kernelVersionFile)
			util.Check(err)
			sharder.kernelVersion = content[0]
		}
	}

	return true
}

// concatResults aggregate all shard files of a given file type by producing
// a concatenated file at the top level of the aggregate results directory.
func (sharder *ShardSchedular) concatResults(filename string) {
	log.Printf("Cancatenating shard file %s", filename)
	file, err := os.Create(sharder.aggDir + filename)
	util.Check(err)
	defer util.Close(file)

	fmt.Fprintf(file, "LTM aggregate file for %s\n", filename)
	fmt.Fprintf(file, "Test run ID %s\n", sharder.testID)
	fmt.Fprintf(file, "Aggregate results from %d shards\n", len(sharder.shards))

	for _, shard := range sharder.shards {
		fmt.Fprintf(file, "\n============SHARD %s============\n", shard.shardID)
		fmt.Fprintf(file, "============CONFIG: %s\n\n", shard.config)
		shardFile := fmt.Sprintf("%s%s/%s", sharder.aggDir, shard.shardID, filename)
		if util.FileExists(shardFile) {
			sourceFile, err := os.Open(shardFile)
			util.Check(err)
			_, err = io.Copy(file, sourceFile)
			util.Check(err)

			util.Close(sourceFile)
		} else {
			fmt.Fprintf(file, "Could not open/read file %s for shard %s\n", filename, shard.shardID)
		}
		fmt.Fprintf(file, "\n==========END SHARD %s==========\n", shard.shardID)
	}
}

// createInfo creates an ltm-info file and a ltm_logs directory.
func (sharder *ShardSchedular) createInfo() {
	log.Printf("Creating LTM info")
	ltmLogDir := sharder.aggDir + "ltm_logs/"
	util.CreateDir(ltmLogDir)

	file, err := os.Create(sharder.aggDir + "ltm-info")
	util.Check(err)
	defer util.Close(file)

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
	//TODO: fix the tricky log dir moving around stuffs

}

func (sharder *ShardSchedular) createRunStats() {
	log.Printf("Creating LTM run stats")
	ltmStatsDir := sharder.aggDir + "ltm-run-stats"
	file, err := os.Create(ltmStatsDir)
	util.Check(err)
	defer util.Close(file)

	fmt.Fprintf(file, "TESTRUNID: %s-%s\n", LTMUserName, sharder.testID)
	fmt.Fprintf(file, "CMDLINE: %s\n", sharder.origCmd)

}

// genResultsSummary call a python script to generate the summary on junit xml test results.
func genResultsSummary(resultsDir string, outputFile string) {
	cmd := exec.Command(genResultsSummaryPath, resultsDir, "--output_file", outputFile)
	log.Printf("%+v", cmd)
	util.CheckRun(cmd, util.RootDir, util.EmptyEnv, os.Stdout, os.Stderr)
}

func (sharder *ShardSchedular) emailReport() {
	log.Printf("Generating email report")
}

func (sharder *ShardSchedular) packResults() {
	log.Printf("Packing aggregated files")

	cmd1 := exec.Command("tar", "-cf", sharder.aggFile+".tar", "-C", sharder.aggDir, ".")
	util.CheckRun(cmd1, util.RootDir, util.EmptyEnv, os.Stdout, os.Stderr)

	cmd2 := exec.Command("xz", "-6ef", sharder.aggFile+".tar")
	util.CheckRun(cmd2, util.RootDir, util.EmptyEnv, os.Stdout, os.Stderr)

	log.Printf("Uploading repacked results tarball")

	gsPath := fmt.Sprintf("%s/results.%s-%s.%s.tar.xz", sharder.bucketSubdir, LTMUserName, sharder.testID, sharder.kernelVersion)
	sharder.gce.UploadFile(sharder.aggFile+".tar.xz", gsPath)

	config := util.GetConfig()
	if config.Get("GCE_UPLOAD_SUMMARY") != "" {
		gsPath = fmt.Sprintf("%s/summary.%s-%s.%s.txt", sharder.bucketSubdir, LTMUserName, sharder.testID, sharder.kernelVersion)
		sharder.gce.UploadFile(sharder.aggDir+"summary", gsPath)
	}
}

func (sharder *ShardSchedular) cleanup() {
	log.Printf("Cleanning up sharder resources")
	util.RemoveDir(sharder.aggDir)

	if strings.HasSuffix(sharder.gsKernel, "-onerun") {
		// TODO: validate this approach
		sharder.gce.DeleteFiles(sharder.gsKernel)
	}
}
