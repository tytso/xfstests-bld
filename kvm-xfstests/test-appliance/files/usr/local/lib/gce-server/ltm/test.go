package main

import (
	"bufio"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"

	"gce-server/util"

	"google.golang.org/api/compute/v1"
)

type MockSharder struct {
	TestID  string
	ProjID  string
	OrigCmd string

	GitRepo  string
	CommitID string
	CallKCS  bool

	Zone           string
	Region         string
	GsBucket       string
	BucketSubdir   string
	GsKernel       string
	KernelVersion  string
	ReportReceiver string
	MaxShards      int
	KeepDeadVM     bool

	LogDir  string
	LogFile string
	AggDir  string
	AggFile string

	ValidArgs []string
	Configs   []string
	Shards    []MockShard
}

type MockShard struct {
	ShardID            string
	Name               string
	Zone               string
	Config             string
	Args               []string
	LogPath            string
	CmdLogPath         string
	SerialOutputPath   string
	ResultsName        string
	TmpResultsDir      string
	UnpackedResultsDir string
	VMTimeout          bool
}

func (sharder *ShardSchedular) Dump(filename string) {
	mock := MockSharder{
		TestID:  sharder.testID,
		ProjID:  sharder.projID,
		OrigCmd: sharder.origCmd,

		GitRepo:  sharder.gitRepo,
		CommitID: sharder.commitID,
		CallKCS:  sharder.callKCS,

		Zone:           sharder.zone,
		Region:         sharder.region,
		GsBucket:       sharder.gsBucket,
		BucketSubdir:   sharder.bucketSubdir,
		GsKernel:       sharder.gsKernel,
		KernelVersion:  sharder.kernelVersion,
		ReportReceiver: sharder.reportReceiver,
		MaxShards:      sharder.maxShards,
		KeepDeadVM:     sharder.keepDeadVM,

		LogDir:  sharder.logDir,
		LogFile: sharder.logFile,
		AggDir:  sharder.aggDir,
		AggFile: sharder.aggFile,

		ValidArgs: sharder.validArgs,
		Configs:   sharder.configs,
	}
	for _, shard := range sharder.shards {
		mock.Shards = append(mock.Shards, shard.Dump())
	}

	file, _ := json.MarshalIndent(mock, "", "\t")
	ioutil.WriteFile(filename, file, 0644)
}

func (shard *ShardWorker) Dump() MockShard {
	return MockShard{
		ShardID:            shard.shardID,
		Name:               shard.name,
		Zone:               shard.zone,
		Config:             shard.config,
		Args:               shard.args,
		LogPath:            shard.logPath,
		CmdLogPath:         shard.cmdLogPath,
		SerialOutputPath:   shard.serialOutputPath,
		ResultsName:        shard.resultsName,
		TmpResultsDir:      shard.tmpResultsDir,
		UnpackedResultsDir: shard.unpackedResultsDir,
		VMTimeout:          shard.vmTimeout,
	}
}

func (mock MockShard) Read(sharder *ShardSchedular) *ShardWorker {
	shard := ShardWorker{
		sharder:            sharder,
		shardID:            mock.ShardID,
		name:               mock.Name,
		zone:               mock.Zone,
		config:             mock.Config,
		args:               mock.Args,
		logPath:            mock.LogPath,
		cmdLogPath:         mock.CmdLogPath,
		serialOutputPath:   mock.SerialOutputPath,
		resultsName:        mock.ResultsName,
		tmpResultsDir:      mock.TmpResultsDir,
		unpackedResultsDir: mock.UnpackedResultsDir,
		vmTimeout:          mock.VMTimeout,
	}
	return &shard
}

func ReadSharder(filename string) *ShardSchedular {
	file, _ := ioutil.ReadFile(filename)

	var mock MockSharder
	json.Unmarshal(file, &mock)

	sharder := ShardSchedular{
		testID:  mock.TestID,
		projID:  mock.ProjID,
		origCmd: mock.OrigCmd,

		gitRepo:  mock.GitRepo,
		commitID: mock.CommitID,
		callKCS:  mock.CallKCS,

		zone:           mock.Zone,
		region:         mock.Region,
		gsBucket:       mock.GsBucket,
		bucketSubdir:   mock.BucketSubdir,
		gsKernel:       mock.GsKernel,
		kernelVersion:  mock.KernelVersion,
		reportReceiver: mock.ReportReceiver,
		maxShards:      mock.MaxShards,
		keepDeadVM:     mock.KeepDeadVM,

		logDir:  mock.LogDir,
		logFile: mock.LogFile,
		aggDir:  mock.AggDir,
		aggFile: mock.AggFile,

		validArgs: mock.ValidArgs,
		configs:   mock.Configs,
	}

	sharder.gce, _ = util.NewGceService(sharder.gsBucket)
	for _, mockShard := range mock.Shards {
		sharder.shards = append(sharder.shards, mockShard.Read(&sharder))
	}
	return &sharder
}

var repo *util.Repository

func test() {
	reader := bufio.NewReader(os.Stdin)
	for true {
		arg, _ := reader.ReadString('\n')
		switch arg[:len(arg)-1] {
		case "clone":
			repo, _ = util.Clone("https://github.com/XiaoyangShen/spinner_test.git", "master")
		case "commit":
			id, _ := repo.GetCommit()
			log.Println(id)
		case "pull":
			repo.Pull()
		case "watch":
			repo.Watch()
		}
	}
}

func test1() {
	reader := bufio.NewReader(os.Stdin)
	for true {
		arg, _ := reader.ReadString('\n')

		validArg, configs, _ := util.ParseCmd(arg[:len(arg)-1])
		log.Printf("%s; %+v\n", validArg, configs)
	}
}

func test2() {
	gce, _ := util.NewGceService("xfstests-xyshen")
	info, _ := gce.GetInstanceInfo("gce-xfstests-bldsrv", "us-central1-f", "xfstests-ltm")
	log.Printf("%+v", info.Metadata)
	for _, item := range info.Metadata.Items {
		log.Printf("%+v", item)
	}

	val := "ahaah"
	newMetadata := compute.Metadata{
		Fingerprint: info.Metadata.Fingerprint,
		Items: []*compute.MetadataItems{
			{
				Key:   "shutdown_reason",
				Value: &val,
			},
		},
	}
	gce.SetMetadata("gce-xfstests-bldsrv", "us-central1-f", "xfstests-ltm", &newMetadata)
}

func test3() {
	sharder := ReadSharder("/root/mock_sharder.json")
	for _, shard := range sharder.shards {
		shard.finish()
	}
	sharder.finish()
}

func test4() {
	config, _ := util.GetConfig(util.KcsConfigFile)
	log.Printf("%+v", config)

	config, _ = util.GetConfig(util.GceConfigFile)
	log.Printf("%+v", config)
}

func test5() {
	util.SendEmail("test email", "xyshen@google.com", util.GceConfigFile)
}

func test6() {
	msg := "random msg"
	content, _ := ioutil.ReadFile("/var/log/go/go.log")
	msg = msg + "\n" + string(content)
	util.SendEmail("test", msg, "xyshen@google.com")
}
