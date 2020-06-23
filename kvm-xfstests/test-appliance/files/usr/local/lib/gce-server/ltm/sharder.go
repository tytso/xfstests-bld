package main

import (
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"example.com/gce-server/util"
)

type shardSchedular struct {
	testID       string
	projID       string
	zone         string
	region       string
	gsBucket     string
	bucketSubdir string
	gsKernel     string
	logDir       string
	validArgs    []string
	configs      []string
	maxShards    int
	keepDeadVM   bool
	gce          util.GceService
	shards       []*shardWorker
}

type SharderInfo struct {
	NumShards int         `json:"num_shards"`
	ShardInfo []ShardInfo `json:"shard_info"`
	ID        string      `json:"id"`
}

func NewShardSchedular(origCmd string, testID string, logDir string, bucketSubdir string,
	gsKernel string, regionShard bool, maxShards int) *shardSchedular {
	config := util.GetConfig()
	// assume a zone looks like us-central1-f and a region looks like us-central1
	// syntax might change in the future so should add support to query for it
	zone := config.Get("GCE_ZONE")
	region := zone[:len(zone)-2]
	keepDeadVM := false
	if config.Get("GCE_LTM_KEEP_DEAD_VM") != "" {
		keepDeadVM = true
	}
	sharder := shardSchedular{
		testID:       testID,
		projID:       config.Get("GCE_PROJECT"),
		zone:         zone,
		region:       region,
		gsBucket:     config.Get("GS_BUCKET"),
		bucketSubdir: bucketSubdir,
		gsKernel:     gsKernel,
		logDir:       logDir,
		maxShards:    maxShards,
		keepDeadVM:   keepDeadVM,
		gce:          util.NewGceService(),
	}

	sharder.validArgs, sharder.configs = getConfigs(origCmd)

	if regionShard {
		sharder.initRegionSharding()
	} else {
		sharder.initLocalSharding()
	}

	return &sharder
}

// create shards in the same zone the VM runs in
func (sharder *shardSchedular) initLocalSharding() {
	allShards := []*shardWorker{}
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

// create shards among all zones with available quotas
func (sharder *shardSchedular) initRegionSharding() {
	allShards := []*shardWorker{}
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
	rand.Shuffle(len(usedZones), func(i, j int) { usedZones[i], usedZones[j] = usedZones[j], usedZones[i] })

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

func splitConfigs(numShards int, configs []string) []string {
	if numShards <= 0 || len(configs) <= numShards {
		return configs
	}

	// split configs among shards with a round-robin pattern
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

func (sharder *shardSchedular) Run() {
	var wg sync.WaitGroup

	for _, shard := range sharder.shards {
		wg.Add(1)
		log.Printf("run shard %+v\n", shard)
		go shard.Run(&wg)
		time.Sleep(500 * time.Millisecond)
	}
	wg.Wait()

	log.Printf("all shards finished")
}

func (sharder *shardSchedular) Info() SharderInfo {
	info := SharderInfo{
		NumShards: len(sharder.shards),
		ID:        sharder.testID,
	}

	for i, shard := range sharder.shards {
		info.ShardInfo = append(info.ShardInfo, shard.Info(i))
	}

	return info
}
