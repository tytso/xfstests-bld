/*
Package gcp deals with Google Cloud APIs and config file parsing.

Files included in this package:
	gcp.go: 	Interface for GCP manipulation.
	config.go: 	Parse config files into dicts.
*/
package gcp

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"thunk.org/gce-server/util/mymath"

	"cloud.google.com/go/storage"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

// Service holds the API clients for Google Cloud Platform.
type Service struct {
	ctx     context.Context
	cancel  context.CancelFunc
	service *compute.Service
	bucket  *storage.BucketHandle
}

// Quota holds the quota limits for a zone.
type Quota struct {
	Zone     string
	cpuLimit int
	ipLimit  int
	ssdLimit int
}

// NewService launches a new GCP service client.
// If gsBucket is not empty, launches a new GS client as well.
func NewService(gsBucket string) (*Service, error) {
	gce := Service{}
	gce.ctx, gce.cancel = context.WithCancel(context.Background())
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

	if gsBucket != "" {
		gce.bucket = gsclient.Bucket(gsBucket)
		_, err = gce.bucket.Attrs(gce.ctx)
		if err != nil {
			return nil, err
		}
	}
	return &gce, nil
}

// Close cancels the context to close the GCP service client.
func (gce *Service) Close() {
	gce.cancel()
}

// GetSerialPortOutput returns the serial port output for an instance.
// Requires the starting offset of desired output. Returns the new offset.
func (gce *Service) GetSerialPortOutput(projID string, zone string, instance string, start int64) (*compute.SerialPortOutput, error) {
	call := gce.service.Instances.GetSerialPortOutput(projID, zone, instance)
	call = call.Start(start)
	return call.Context(gce.ctx).Do()
}

// GetInstanceInfo returns the info about an instance.
func (gce *Service) GetInstanceInfo(projID string, zone string, instance string) (*compute.Instance, error) {
	return gce.service.Instances.Get(projID, zone, instance).Context(gce.ctx).Do()
}

// SetMetadata sets the metadata for an instance.
func (gce *Service) SetMetadata(projID string, zone string, instance string, metadata *compute.Metadata) error {
	_, err := gce.service.Instances.SetMetadata(projID, zone, instance, metadata).Context(gce.ctx).Do()
	return err
}

// DeleteInstance deletes an instance.
func (gce *Service) DeleteInstance(projID string, zone string, instance string) error {
	_, err := gce.service.Instances.Delete(projID, zone, instance).Context(gce.ctx).Do()
	return err
}

func (gce *Service) getRegionInfo(projID string, region string) (*compute.Region, error) {
	return gce.service.Regions.Get(projID, region).Context(gce.ctx).Do()
}

func (gce *Service) getAllRegionsInfo(projID string) ([]*compute.Region, error) {
	allRegions := []*compute.Region{}
	req := gce.service.Regions.List(projID)
	err := req.Pages(gce.ctx, func(page *compute.RegionList) error {
		allRegions = append(allRegions, page.Items...)
		return nil
	})
	return allRegions, err
}

func (gce *Service) getZoneInfo(projID string, zone string) (*compute.Zone, error) {
	return gce.service.Zones.Get(projID, zone).Context(gce.ctx).Do()
}

// GetRegionQuota picks the first available zone in a region and returns the
// quota limits on it.
// Every shard needs 2 vCPUs and SSD space of GCE_MIN_SCR_SIZE.
// SSD space is no less than 50 GB.
func (gce *Service) GetRegionQuota(projID string, region string) (*Quota, error) {
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
	size, err := GceConfig.Get("GCE_MIN_SCR_SIZE")
	if err != nil {
		return nil, err
	}
	ssdMin, err := strconv.Atoi(size)
	if err != nil {
		ssdMin = 0
	}
	ssdLimit := ssdNum / mymath.MaxInt(50, ssdMin)

	return &Quota{
		Zone:     pickedZone,
		cpuLimit: cpuNum / 2,
		ipLimit:  ipNum,
		ssdLimit: ssdLimit,
	}, nil
}

// GetAllRegionsQuota returns quota limits for every available region.
func (gce *Service) GetAllRegionsQuota(projID string) ([]*Quota, error) {
	allRegions, err := gce.getAllRegionsInfo(projID)
	if err != nil {
		return []*Quota{}, err
	}
	quotas := []*Quota{}
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
func (quota *Quota) GetMaxShard() (int, error) {
	return mymath.MinIntSlice([]int{quota.cpuLimit, quota.ipLimit, quota.ssdLimit})
}

// getFiles returns an iterator for all files with a matching path prefix on GS.
func (gce *Service) getFiles(prefix string) (*storage.ObjectIterator, error) {
	if gce.bucket == nil {
		return nil, fmt.Errorf("GS client is not initialized")
	}
	query := &storage.Query{Prefix: prefix}
	it := gce.bucket.Objects(gce.ctx, query)
	return it, nil
}

// DeleteFiles removes all files with a matching path prefix on GS.
func (gce *Service) DeleteFiles(prefix string) (int, error) {
	it, err := gce.getFiles(prefix)
	if err != nil {
		return 0, err
	}

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
func (gce *Service) GetFileNames(prefix string) ([]string, error) {
	names := []string{}
	it, err := gce.getFiles(prefix)
	if err != nil {
		return names, err
	}

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
func (gce *Service) UploadFile(localPath string, gsPath string) error {
	if gce.bucket == nil {
		return fmt.Errorf("GS client is not initialized")
	}
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

// NotFound returns true if err is 404 not found.
func NotFound(err error) bool {
	if err != nil {
		if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
			return true
		}
	}
	return false
}

func (gce *Service) ResetVM(project string, zone string, instance string) error {
        instancesService := compute.NewInstancesService(gce.service)
        call := instancesService.Reset(project, zone, instance)
        _, err := call.Do()
        return err
}
