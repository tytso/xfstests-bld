package main

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"thunk.org/gce-server/util/check"
	"thunk.org/gce-server/util/gcp"
	"thunk.org/gce-server/util/git"
	"thunk.org/gce-server/util/logging"
	"thunk.org/gce-server/util/mymath"
	"thunk.org/gce-server/util/server"

	"github.com/sirupsen/logrus"
	"google.golang.org/api/compute/v1"
)

var (
	buildTimeout = 1 * time.Hour

	newBuild  = make(chan struct{}, 1)
	buildLock sync.Mutex

	trackerMu    sync.Mutex
	activeTasks  int
	taskWG       sync.WaitGroup
	shuttingDown bool
	lastActivity = time.Now()
	shutdownFunc = shutdown
)

func notifyNewBuild() {
	select {
	case newBuild <- struct{}{}:
	default:
	}
}

// TryBeginTask registers an in-flight build or bisect task unless KCS is
// already shutting down.
func TryBeginTask() bool {
	trackerMu.Lock()
	defer trackerMu.Unlock()
	if shuttingDown {
		return false
	}
	activeTasks++
	taskWG.Add(1)
	lastActivity = time.Now()
	notifyNewBuild()
	return true
}

// EndTask marks an in-flight build or bisect task finished and refreshes
// the idle timer baseline.
func EndTask() {
	trackerMu.Lock()
	if activeTasks > 0 {
		activeTasks--
		taskWG.Done()
	}
	lastActivity = time.Now()
	trackerMu.Unlock()
	notifyNewBuild()
}

// WaitTasks waits for all registered build and bisect tasks to finish.
func WaitTasks() {
	taskWG.Wait()
}

func touchBuildActivity() {
	trackerMu.Lock()
	lastActivity = time.Now()
	trackerMu.Unlock()
	notifyNewBuild()
}

func nextTrackerWait() time.Duration {
	trackerMu.Lock()
	defer trackerMu.Unlock()
	if activeTasks > 0 {
		return buildTimeout
	}
	rem := buildTimeout - time.Since(lastActivity)
	if rem <= 0 {
		return buildTimeout
	}
	return rem
}

/*
StartTracker initiates a tracker for KCS server.

If no build task received within buildTimeout and no active bisector,
tracker shuts down the server and delete the VM.
It appends a metadata to VM so that LTM would attempt to launch a new
KCS server only after the shutdown finishes.
*/
func StartTracker(instance *server.Instance, stop <-chan struct{}, finished chan<- struct{}) {
	defer close(finished)
	log := instance.Log()

	touchBuildActivity()

	for alive := true; alive; {
		timer := time.NewTimer(nextTrackerWait())
		select {
		case <-stop:
			timer.Stop()
			alive = false
		case <-newBuild:
			timer.Stop()
		case <-timer.C:
			trackerMu.Lock()
			busy := activeTasks > 0 || time.Since(lastActivity) < buildTimeout
			trackerMu.Unlock()
			if busy {
				continue
			}

			log.Warnf("KCS server has been idle for %s", buildTimeout.Round(time.Minute))
			bisectors := BisectorStatus()
			log.Infof("There are %d active bisectors", len(bisectors))
			if len(bisectors) > 0 || HasActiveOrPendingBisectors() {
				log.Infof("%+v", bisectors)
				touchBuildActivity()
			} else if logging.DEBUG {
				log.Info("DEBUG mode enabled, skipping KCS auto-shutdown")
				touchBuildActivity()
			} else {
				trackerMu.Lock()
				if activeTasks > 0 || time.Since(lastActivity) < buildTimeout || HasActiveOrPendingBisectors() {
					trackerMu.Unlock()
					continue
				}
				shuttingDown = true
				trackerMu.Unlock()

				shutdownFunc(instance)
				alive = false
			}
		}
	}
}

// RunBuild builds the kernel and upload the kernel image.
// It signals the server tracker to reset the timeout timer.
func RunBuild(repo *git.Repository, gsBucket string, gsPath string, gsConfig string, kConfigOpts string, kbuildOpts string, arch string, testID string, buildLog string) error {
	if err := check.ValidateTestID(testID); err != nil {
		return fmt.Errorf("invalid testID: %w", err)
	}
	buildLock.Lock()
	defer buildLock.Unlock()
	touchBuildActivity()
	defer touchBuildActivity()

	file, err := os.Create(buildLog)
	if err != nil {
		return err
	}
	defer file.Close()
	err = repo.BuildUpload(gsBucket, gsPath, gsConfig, arch, kConfigOpts, kbuildOpts, file)

	return err
}

func shutdown(instance *server.Instance) {
	log := instance.Log()
	log.Info("Set metadata before shutdown")

	zone, err := gcp.GceConfig().Get("GCE_ZONE")
	check.Panic(err, log, "Failed to get zone config")
	projID, err := gcp.GceConfig().Get("GCE_PROJECT")
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
