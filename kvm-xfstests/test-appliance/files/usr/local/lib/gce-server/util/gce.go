package util

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

const gceStateDir = "/var/lib/gce-xfstests/"

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

// NewGceService launches a new GceService Client.
func NewGceService(gsBucket string) (*GceService, error) {
	gce := GceService{}
	gce.ctx = context.Background()
	client, err := google.DefaultClient(gce.ctx, compute.CloudPlatformScope)
	if err != nil {
		return nil, err
	}
	s, err := compute.New(client)
	if err != nil {
		return nil, err
	}
	gce.service = s

	gsclient, err := storage.NewClient(gce.ctx)
	if err != nil {
		return nil, err
	}
	gce.bucket = gsclient.Bucket(gsBucket)
	_, err = gce.bucket.Attrs(gce.ctx)
	if err != nil {
		return nil, err
	}
	return &gce, nil
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
func (gce *GceService) SetMetadata(projID string, zone string, instance string, metadata *compute.Metadata) error {
	_, err := gce.service.Instances.SetMetadata(projID, zone, instance, metadata).Context(gce.ctx).Do()
	return err
}

// DeleteInstance deletes an instance.
func (gce *GceService) DeleteInstance(projID string, zone string, instance string) error {
	_, err := gce.service.Instances.Delete(projID, zone, instance).Context(gce.ctx).Do()
	return err
}

func (gce *GceService) getRegionInfo(projID string, region string) (*compute.Region, error) {
	return gce.service.Regions.Get(projID, region).Context(gce.ctx).Do()
}

func (gce *GceService) getAllRegionsInfo(projID string) ([]*compute.Region, error) {
	allRegions := []*compute.Region{}
	req := gce.service.Regions.List(projID)
	err := req.Pages(gce.ctx, func(page *compute.RegionList) error {
		allRegions = append(allRegions, page.Items...)
		return nil
	})
	return allRegions, err
}

func (gce *GceService) getZoneInfo(projID string, zone string) (*compute.Zone, error) {
	return gce.service.Zones.Get(projID, zone).Context(gce.ctx).Do()
}

// GetRegionQuota picks the first available zone in a region and returns the
// quota limits on it.
// Every shard needs 2 vCPUs and SSD space of GCE_MIN_SCR_SIZE.
// SSD space is no less than 50 GB.
func (gce *GceService) GetRegionQuota(projID string, region string) (*GceQuota, error) {
	regionInfo, err := gce.getRegionInfo(projID, region)
	if err != nil {
		return nil, err
	}
	var pickedZone string
	for _, zone := range regionInfo.Zones {
		slice := strings.Split(zone, "/")
		zone := slice[len(slice)-1]
		zoneInfo, err := gce.getZoneInfo(projID, zone)
		if err != nil {
			return nil, err
		}
		if zoneInfo.Status == "UP" {
			pickedZone = zoneInfo.Name
			break
		}
	}
	if pickedZone == "" {
		return nil, fmt.Errorf("GCE region %s has no available zones", region)
	}
	var cpuNum, ipNum, ssdNum int
	config, err := GetConfig(GceConfigFile)
	if err != nil {
		return nil, err
	}
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
	}, nil
}

// GetAllRegionsQuota returns quota limits for every available region.
func (gce *GceService) GetAllRegionsQuota(projID string) ([]*GceQuota, error) {
	allRegions, err := gce.getAllRegionsInfo(projID)
	if err != nil {
		return []*GceQuota{}, err
	}
	quotas := []*GceQuota{}
	for _, region := range allRegions {
		if region.Status == "UP" {
			quota, err := gce.GetRegionQuota(projID, region.Name)
			if quota != nil && err == nil {
				quotas = append(quotas, quota)
			}
		}
	}
	return quotas, nil
}

// GetMaxShard return the max possible number of shards according to the quota limits
func (quota *GceQuota) GetMaxShard() (int, error) {
	return MinIntSlice([]int{quota.cpuLimit, quota.ipLimit, quota.ssdLimit})
}

// GetFiles returns an iterator for all files with a matching path prefix on GS.
func (gce *GceService) GetFiles(prefix string) *storage.ObjectIterator {
	query := &storage.Query{Prefix: prefix}
	it := gce.bucket.Objects(gce.ctx, query)
	return it
}

// DeleteFiles removes all files with a matching path prefix on GS.
func (gce *GceService) DeleteFiles(prefix string) (int, error) {
	it := gce.GetFiles(prefix)
	count := 0
	for {
		objAttrs, err := it.Next()
		if err != nil {
			if err == iterator.Done {
				break
			} else {
				return 0, err
			}
		}
		err = gce.bucket.Object(objAttrs.Name).Delete(gce.ctx)
		if err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

// GetFileNames returns a slice of file names with a matching path prefix on GS.
func (gce *GceService) GetFileNames(prefix string) ([]string, error) {
	it := gce.GetFiles(prefix)
	names := []string{}
	for {
		objAttrs, err := it.Next()
		if err != nil {
			if err == iterator.Done {
				break
			} else {
				return names, err
			}
		}
		names = append(names, objAttrs.Name)
	}
	return names, nil
}

// UploadFile uploads a local file or directory to GS.
func (gce *GceService) UploadFile(localPath string, gsPath string) error {
	obj := gce.bucket.Object(gsPath)
	w := obj.NewWriter(gce.ctx)
	defer w.Close()
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(w, file)
	if err != nil {
		return err
	}

	return nil
}

// IsNotFound returns true if err is 404 not found.
func IsNotFound(err error) bool {
	if err != nil {
		if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
			return true
		}
	}
	return false
}
