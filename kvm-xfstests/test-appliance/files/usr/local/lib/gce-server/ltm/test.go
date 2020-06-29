package main

import (
	"encoding/json"
	"io/ioutil"

	"example.com/gce-server/util"
)

type MockSharder struct {
	TestID  string
	ProjID  string
	OrigCmd string

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

	file, err := json.MarshalIndent(mock, "", "\t")
	util.Check(err)
	err = ioutil.WriteFile(filename, file, 0644)
	util.Check(err)
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
	file, err := ioutil.ReadFile(filename)
	util.Check(err)

	var mock MockSharder
	json.Unmarshal([]byte(file), &mock)

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

	sharder.gce = util.NewGceService(sharder.gsBucket)
	for _, mockShard := range mock.Shards {
		sharder.shards = append(sharder.shards, mockShard.Read(&sharder))
	}
	return &sharder
}
