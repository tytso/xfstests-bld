package main

import (
	"log"

	"example.com/gce-server/util"
)

type shardSchedular struct {
	id           string
	origCmd      string
	projID       string
	zone         string
	region       string
	gsBucket     string
	bucketSubdir string
	gsKernel     string
	logDir       string
	gce          util.GceService
	shards       *[]shard
}

func newShardSchedular(origCmd string, id string, logDir string,
	gsKernel string, regionShard bool, maxShards int) shardSchedular {
	config := util.GetConfig()
	// assume a zone looks like us-central1-f and a region looks like us-central1
	// syntax might change in the future so should add support to query for it
	zone := config.Get("GCE_ZONE")
	region := zone[:len(zone)-2]
	sharder := shardSchedular{
		id:           id,
		origCmd:      origCmd,
		projID:       config.Get("GCE_PROJECT"),
		zone:         zone,
		region:       region,
		gsBucket:     config.Get("GS_BUCKET"),
		bucketSubdir: config.Get("BUCKET_SUBDIR"),
		gsKernel:     gsKernel,
		logDir:       logDir,
		gce:          util.NewGceService(),
	}

	if regionShard {
		sharder.initRegionSharding(maxShards)
	} else {
		sharder.initLocalSharding(maxShards)
	}

	return sharder
}

// create shards in the same zone the VM runs in
func (sharder *shardSchedular) initLocalSharding(maxShards int) {
	// allShards := []*shard{}
	quota := sharder.gce.GetRegionQuota(sharder.projID, sharder.region)
	if quota == nil {
		log.Fatalf("GCE region %s is out of quota\n", sharder.region)
	}
	numShards := quota.GetMaxShard()
	if maxShards > 0 {
		numShards = util.MaxInt(numShards, maxShards)
	}

}

// create shards among all zones with available quotas
func (sharder *shardSchedular) initRegionSharding(maxShards int) {

}

func (sharder *shardSchedular) splitConfigs(numShards int) []string {
	return []string{}
}
