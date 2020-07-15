package main

import (
	"encoding/json"
	"io/ioutil"

	"gce-server/server"
	"gce-server/util"
)

type JsonSharder struct {
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
	Shards    []JsonShard
}

type JsonShard struct {
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
	mock := JsonSharder{
		TestID:  sharder.testID,
		ProjID:  sharder.projID,
		OrigCmd: sharder.origCmd,

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

func (shard *ShardWorker) Dump() JsonShard {
	return JsonShard{
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

func (mock JsonShard) Read(sharder *ShardSchedular) *ShardWorker {
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

	var mock JsonSharder
	json.Unmarshal(file, &mock)

	sharder := ShardSchedular{
		testID:  mock.TestID,
		projID:  mock.ProjID,
		origCmd: mock.OrigCmd,

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

func MockNewShardSchedular(c server.TaskRequest, testID string) *ShardSchedular {
	sharder := ShardSchedular{
		testID:      testID,
		testRequest: c,
		log:         server.Log.WithField("testID", testID),
	}
	if c.ExtraOptions != nil && c.ExtraOptions.Requester == server.KCSBisectStep {
		sharder.reportKCS = true
	}

	return &sharder
}

func (sharder *ShardSchedular) MockRun() {
	sharder.log.Warn("mock test finished")
	if sharder.testRequest.ExtraOptions != nil {
		sharder.log.WithField("result", sharder.testRequest.ExtraOptions.TestResult).Warn("get test results")
	}
	if sharder.reportKCS {
		sharder.testRequest.ExtraOptions.Requester = server.LTMBisectStep
		ForwardKCS(sharder.testRequest, sharder.testID)
	}
}
