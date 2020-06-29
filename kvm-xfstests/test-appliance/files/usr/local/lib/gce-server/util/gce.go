package util

import (
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

const (
	gceStateDir   = "/var/lib/gce-xfstests/"
	gceConfigFile = "/usr/local/lib/gce_xfstests.config"
)

// Config dictionary retrieved from gce_xfstests.config.
type Config struct {
	kv map[string]string
}

// GceService holds the API clients for Google Cloud Platform.
type GceService struct {
	ctx     context.Context
	service *compute.Service
	bucket  *storage.BucketHandle
}

// GceQuota holds the quota limits for a zone.
type GceQuota struct {
	Zone     string
	cpuLimit int
	ipLimit  int
	ssdLimit int
}

// GetConfig reads from the config file and returns a struct Config.
func GetConfig() Config {
	c := Config{make(map[string]string)}
	re := regexp.MustCompile(`declare -- (.*?)="(.*?)"`)

	lines, err := ReadLines(gceConfigFile)
	Check(err)

	for _, line := range lines {
		tokens := re.FindStringSubmatch(line)
		if len(tokens) == 3 {
			c.kv[tokens[1]] = tokens[2]
		}
	}

	return c
}

// Get a certain config value according to key.
// Returns empty string if key is not present in config.
func (c *Config) Get(key string) string {
	if val, ok := c.kv[key]; ok {
		return val
	}
	return ""
}

// NewGceService launches a new GceService Client.
func NewGceService(gsBucket string) GceService {
	gce := GceService{}
	gce.ctx = context.Background()
	client, err := google.DefaultClient(gce.ctx, compute.CloudPlatformScope)
	Check(err)
	s, err := compute.New(client)
	Check(err)
	gce.service = s

	gsclient, err := storage.NewClient(gce.ctx)
	Check(err)
	gce.bucket = gsclient.Bucket(gsBucket)
	_, err = gce.bucket.Attrs(gce.ctx)
	Check(err)
	return gce
}

// GetSerialPortOutput returns the serial port output for an instance.
// Requires the starting offset of desired output. Returns the new offset.
func (gce *GceService) GetSerialPortOutput(projID string, zone string, instance string, start int64) (*compute.SerialPortOutput, error) {
	call := gce.service.Instances.GetSerialPortOutput(projID, zone, instance)
	call = call.Start(start)
	return call.Context(gce.ctx).Do()
}

// GetInstanceInfo returns the info about an instance.
func (gce *GceService) GetInstanceInfo(projID string, zone string, instance string) (*compute.Instance, error) {
	return gce.service.Instances.Get(projID, zone, instance).Context(gce.ctx).Do()
}

// SetMetadata sets the metadata for an instance.
func (gce *GceService) SetMetadata(projID string, zone string, instance string, metadata *compute.Metadata) {
	_, err := gce.service.Instances.SetMetadata(projID, zone, instance, metadata).Context(gce.ctx).Do()
	Check(err)
}

// DeleteInstance deletes an instance.
func (gce *GceService) DeleteInstance(projID string, zone string, instance string) {
	_, err := gce.service.Instances.Delete(projID, zone, instance).Context(gce.ctx).Do()
	Check(err)
}

func (gce *GceService) getRegionInfo(projID string, region string) *compute.Region {
	resp, err := gce.service.Regions.Get(projID, region).Context(gce.ctx).Do()
	Check(err)
	return resp
}

func (gce *GceService) getAllRegionsInfo(projID string) []*compute.Region {
	allRegions := []*compute.Region{}
	req := gce.service.Regions.List(projID)
	err := req.Pages(gce.ctx, func(page *compute.RegionList) error {
		allRegions = append(allRegions, page.Items...)
		return nil
	})
	Check(err)
	return allRegions
}

func (gce *GceService) getZoneInfo(projID string, zone string) *compute.Zone {
	resp, err := gce.service.Zones.Get(projID, zone).Context(gce.ctx).Do()
	Check(err)
	return resp
}

// GetRegionQuota picks the first available zone in a region and returns the
// quota limits on it.
// Every shard needs 2 vCPUs and SSD space of GCE_MIN_SCR_SIZE.
// SSD space is no less than 50 GB.
func (gce *GceService) GetRegionQuota(projID string, region string) *GceQuota {
	regionInfo := gce.getRegionInfo(projID, region)
	var pickedZone string
	for _, zone := range regionInfo.Zones {
		slice := strings.Split(zone, "/")
		zone := slice[len(slice)-1]
		zoneInfo := gce.getZoneInfo(projID, zone)
		if zoneInfo.Status == "UP" {
			pickedZone = zoneInfo.Name
			break
		}
	}
	if pickedZone == "" {
		log.Printf("GCE region %s has no available zones\n", region)
		return nil
	}
	var cpuNum, ipNum, ssdNum int
	config := GetConfig()
	for _, quota := range regionInfo.Quotas {
		switch quota.Metric {
		case "CPUS":
			cpuNum = int(quota.Limit - quota.Usage)
		case "IN_USE_ADDRESSES":
			ipNum = int(quota.Limit - quota.Usage)
		case "SSD_TOTAL_GB":
			ssdNum = int(quota.Limit - quota.Usage)
		}
	}
	ssdMin, err := strconv.Atoi(config.Get("GCE_MIN_SCR_SIZE"))
	if err != nil {
		ssdMin = 0
	}
	ssdLimit := ssdNum / MaxInt(50, ssdMin)

	return &GceQuota{
		Zone:     pickedZone,
		cpuLimit: cpuNum / 2,
		ipLimit:  ipNum,
		ssdLimit: ssdLimit,
	}
}

// GetAllRegionsQuota returns quota limits for every availble region.
func (gce *GceService) GetAllRegionsQuota(projID string) []*GceQuota {
	allRegions := gce.getAllRegionsInfo(projID)
	quotas := []*GceQuota{}
	for _, region := range allRegions {
		if region.Status == "UP" {
			quota := gce.GetRegionQuota(projID, region.Name)
			if quota != nil {
				quotas = append(quotas, quota)
			}
		}
	}
	return quotas
}

// GetMaxShard return the max possible number of shards according to the quota limits
func (quota *GceQuota) GetMaxShard() int {
	maxShard, err := MinIntSlice([]int{quota.cpuLimit, quota.ipLimit, quota.ssdLimit})
	Check(err)
	return maxShard
}

// GetFiles returns an iterator for all files with a matching path prefix on GS.
func (gce *GceService) GetFiles(prefix string) *storage.ObjectIterator {
	query := &storage.Query{Prefix: prefix}
	it := gce.bucket.Objects(gce.ctx, query)
	return it
}

// DeleteFiles removes all files with a matching path prefix on GS.
func (gce *GceService) DeleteFiles(prefix string) int {
	it := gce.GetFiles(prefix)
	count := 0
	for {
		objAttrs, err := it.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			Check(err)
		}
		err = gce.bucket.Object(objAttrs.Name).Delete(gce.ctx)
		Check(err)
		count++
	}
	return count
}

// GetFileNames returns a slice of file names with a matching path prefix on GS.
func (gce *GceService) GetFileNames(prefix string) []string {
	it := gce.GetFiles(prefix)
	names := []string{}
	for {
		objAttrs, err := it.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			Check(err)
		}
		names = append(names, objAttrs.Name)
	}
	return names
}

// UploadFile uploads a local file or directory to GS.
func (gce *GceService) UploadFile(localPath string, gsPath string) {
	obj := gce.bucket.Object(gsPath)
	w := obj.NewWriter(gce.ctx)
	file, err := os.Open(localPath)
	Check(err)

	defer Close(file)
	_, err = io.Copy(w, file)
	Check(err)

	err = w.Close()
	Check(err)
}

// IsNotFound returns true if err is a 404 not found error.
func IsNotFound(err error) bool {
	if err != nil {
		if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
			return true
		}
	}
	return false
}
