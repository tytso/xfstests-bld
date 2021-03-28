package main

import (
	"os"
	"os/exec"
	"sync"
	"time"

	"gce-server/util/check"
	"gce-server/util/gcp"
	"gce-server/util/git"
	"gce-server/util/logging"
	"gce-server/util/mymath"
	"gce-server/util/server"

	"github.com/sirupsen/logrus"
	"google.golang.org/api/compute/v1"
)

const (
	buildTimeout = 1 * time.Hour
)

var (
	newBuild  chan bool
	buildLock sync.Mutex
)

/*
StartTracker initiates a tracker for KCS server.

If no build task received within buildTimeout and no active bisector,
tracker shuts down the server and delete the VM.
It appends a metadata to VM so that LTM would attempt to launch a new
KCS server only after the shutdown finishes.
*/
func StartTracker(instance *server.Instance, finished chan bool) {
	newBuild = make(chan bool)
	log := instance.Log()

	for alive := true; alive; {
		timer := time.NewTimer(buildTimeout)
		select {
		case <-newBuild:
			timer.Stop()
		case <-timer.C:
			log.Warnf("KCS server has been idle for %s", buildTimeout.Round(time.Minute))
			bisectors := BisectorStatus()
			log.Infof("There are %d active bisectors", len(bisectors))
			if len(bisectors) > 0 {
				log.Infof("%+v", bisectors)
				timer.Stop()
			} else {
				if !logging.DEBUG {
					shutdown(instance)
				}
				alive = false
			}
		}
	}
	finished <- true
}

// RunBuild builds the kernel and upload the kernel image.
// It signals the server tracker to reset the timeout timer.
func RunBuild(repo *git.Repository, gsBucket string, gsPath string, gsConfig string, kConfigOpts string, testID string, buildLog string) error {
	buildLock.Lock()
	defer buildLock.Unlock()
	newBuild <- true

	file, err := os.Create(buildLog)
	if err != nil {
		return err
	}
	defer file.Close()
	err = repo.BuildUpload(gsBucket, gsPath, gsConfig, kConfigOpts, file)

	return err
}

func shutdown(instance *server.Instance) {
	log := instance.Log()
	log.Info("Set metadata before shutdown")

	zone, err := gcp.GceConfig.Get("GCE_ZONE")
	check.Panic(err, log, "Failed to get zone config")
	projID, err := gcp.GceConfig.Get("GCE_PROJECT")
	check.Panic(err, log, "Failed to get project config")

	gce, err := gcp.NewService("")
	check.Panic(err, log, "Failed to connect to GCE service")
	defer gce.Close()

	instanceInfo, err := gce.GetInstanceInfo(projID, zone, server.KCSServer)
	check.Panic(err, log, "Failed to get KCS instance info")

	metadata := instanceInfo.Metadata
	val := "KCS keeps idle"
	metadata.Items = append(metadata.Items, &compute.MetadataItems{
		Key:   "shutdown_reason",
		Value: &val,
	})
	err = gce.SetMetadata(projID, zone, server.KCSServer, metadata)
	check.NoError(err, log, "Failed to set VM metadata")
	time.Sleep(1 * time.Second)

	saveLog(log)
	instance.Shutdown()

	log.Info("Delete KCS VM")
	err = gce.DeleteInstance(projID, zone, server.KCSServer)
	check.Panic(err, log, "Failed to delete KCS itself")
}

func saveLog(log *logrus.Entry) {
	log.Info("Save KCS server log to cache disk")

	timestamp := mymath.GetTimeStamp()
	err := check.CreateDir(logging.KCSCachedDir)
	if !check.NoError(err, log, "Failed to create dir") {
		return
	}
	logging.Sync(log)
	cmd := exec.Command("tar", "-cf", logging.KCSCachedDir+timestamp+".tar", "-C", logging.LogDir, ".")
	cmdLog := log.WithField("cmd", cmd.Args)
	w := cmdLog.Writer()
	defer w.Close()
	err = check.Run(cmd, check.RootDir, check.EmptyEnv, w, w)
	check.NoError(err, log, "Failed to create log tarball")
}
