package util

import (
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

const (
	gceStateDir   = "/var/lib/gce-xfstests/"
	gceConfigFile = "/usr/local/lib/gce_xfstests.config"
)

type Config struct {
	kv map[string]string
}

type GceService struct {
	ctx     context.Context
	service *compute.Service
}

type GceQuota struct {
	Zone     string
	cpuLimit int
	ipLimit  int
	ssdLimit int
}

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

func (c *Config) Get(key string) string {
	if val, ok := c.kv[key]; ok {
		return val
	}
	return ""
}

func NewGceService() GceService {
	gce := GceService{}
	gce.ctx = context.Background()
	c, err := google.DefaultClient(gce.ctx, compute.CloudPlatformScope)
	Check(err)
	s, err := compute.New(c)
	Check(err)
	gce.service = s
	return gce
}

func (gce *GceService) GetSerialPortOutput(projID string, zone string, instance string, start int64) *compute.SerialPortOutput {
	call := gce.service.Instances.GetSerialPortOutput(projID, zone, instance)
	call = call.Start(start)
	resp, err := call.Context(gce.ctx).Do()
	Check(err)
	return resp
}

func (gce *GceService) GetInstanceInfo(projID string, zone string, instance string) (*compute.Instance, error) {
	return gce.service.Instances.Get(projID, zone, instance).Context(gce.ctx).Do()
}

func (gce *GceService) SetMetadata(projID string, zone string, instance string, metadata *compute.Metadata) {
	_, err := gce.service.Instances.SetMetadata(projID, zone, instance, metadata).Context(gce.ctx).Do()
	Check(err)
}

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

func (gce *GceService) GetRegionQuota(projID string, region string) *GceQuota {
	regionInfo := gce.getRegionInfo(projID, region)
	var pickedZone string
	// pick the first zone that is up
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
	Check(err)
	ssdLimit := ssdNum / MaxInt(50, ssdMin)

	return &GceQuota{
		Zone:     pickedZone,
		cpuLimit: cpuNum / 2,
		ipLimit:  ipNum,
		ssdLimit: ssdLimit,
	}
}

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

func (quota *GceQuota) GetMaxShard() int {
	maxShard, err := MinIntSlice([]int{quota.cpuLimit, quota.ipLimit, quota.ssdLimit})
	Check(err)
	return maxShard
}

func IsNotFound(err error) bool {
	if err != nil {
		if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
			return true
		}
	}
	return false
}
