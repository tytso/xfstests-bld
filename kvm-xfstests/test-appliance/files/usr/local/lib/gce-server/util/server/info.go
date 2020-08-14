package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gce-server/util/check"
	"gce-server/util/gcp"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
)

// SharderInfo exports sharder info.
type SharderInfo struct {
	ID        string      `json:"id"`
	Command   string      `json:"command"`
	NumShards int         `json:"num_shards"`
	Result    string      `json:"test_result"`
	ShardInfo []ShardInfo `json:"shards"`
}

func (s SharderInfo) String() string {
	info := fmt.Sprintf(
		"============SHARDER INFO %s============\nCMDLINE:\t%s\nSHARD NUM:\t%d\nTEST RESULT:\t%s\n",
		s.ID,
		s.Command,
		s.NumShards,
		s.Result,
	)
	for _, shard := range s.ShardInfo {
		info += shard.String()
	}
	return info
}

// ShardInfo exports shard info.
type ShardInfo struct {
	ID     string `json:"id"`
	Config string `json:"cfg"`
	Zone   string `json:"zone"`
	Status string `json:"vm_status"`
	Time   string `json:"since_update"`
	Result string `json:"test_result"`
}

func (s ShardInfo) String() string {
	return fmt.Sprintf(
		"------------SHARD INFO %s------------\n\tCONFIG:\t%s\n\tZONE:\t%s\n\tVM STATUS:\t%s\n\tSINCE LAST UPDATE:\t%s\n\tTEST STATUS:\t%s\n",
		s.ID,
		s.Config,
		s.Zone,
		s.Status,
		s.Time,
		s.Result,
	)
}

// WatcherInfo exports watcher info.
type WatcherInfo struct {
	ID      string   `json:"id"`
	Command string   `json:"command"`
	Repo    string   `json:"repo"`
	Branch  string   `json:"branch"`
	HEAD    string   `json:"HEAD"`
	Tests   []string `json:"active_tests"`
	Packs   []string `json:"packed_tests"`
}

func (w WatcherInfo) String() string {
	return fmt.Sprintf(
		"============WATCHER INFO %s============\nCMDLINE:\t%s\nREPO:\t%s\nBRANCH:\t%s\nHEAD:\t%s\nACTIVE TESTS:\n\t%s\nPACKED TESTS:\n\t%s\n",
		w.ID,
		w.Command,
		w.Repo,
		w.Branch,
		w.HEAD,
		strings.Join(w.Tests, "\n\t"),
		strings.Join(w.Packs, "\t\n"),
	)
}

// BisectorInfo exports bisector info.
type BisectorInfo struct {
	ID          string   `json:"id"`
	Command     string   `json:"command"`
	Repo        string   `json:"repo"`
	BadCommit   string   `json:"bad_commit"`
	GoodCommits []string `json:"good_commits"`
	LastActive  string   `json:"last_active"`
	Log         []string `json:"log"`
}

func (b BisectorInfo) String() string {
	return fmt.Sprintf(
		"============BISECTOR INFO %s============\nCMDLINE:\t%s\nREPO:\t%s\nBAD COMMIT:\t%s\nGOOD COMMITS:\t%s\nSINCE LAST UPDATE:\t%s\nBISECT LOG:\n%s\n",
		b.ID,
		b.Command,
		b.Repo,
		b.BadCommit,
		strings.Join(b.GoodCommits, ", "),
		b.LastActive,
		strings.Join(b.Log, "\n"),
	)
}

// StatusResponse returns the running status to user.
type StatusResponse struct {
	Sharders  []SharderInfo  `json:"sharders"`
	Watchers  []WatcherInfo  `json:"watchers"`
	Bisectors []BisectorInfo `json:"bisectors"`
}

// InternalQuery sends a query request from LTM to KCS.
// It returns running status from KCS if KCS is running,
// and returns empty response if it's not.
func InternalQuery(log *logrus.Entry) StatusResponse {
	log.Info("Sending status query to KCS")

	active := accessKCS(log, false)
	if !active {
		log.Info("KCS is not running, skipping query")
		return StatusResponse{}
	}
	gcp.Update()

	config := gcp.KCSConfig
	if config == nil {
		log.Panicf("Failed to get config")
	}
	ip, err := config.Get("GCE_KCS_INT_IP")
	check.Panic(err, log, "Failed to get ip")

	url := fmt.Sprintf("https://%s/internal-status", ip)

	c := TaskRequest{
		ExtraOptions: &InternalOptions{
			Requester: Query,
			Password:  password,
		},
	}

	js, err := json.Marshal(c)
	check.Panic(err, log, "Failed to encode json request body")
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(js))
	check.Panic(err, log.WithField("js", js), "Failed to format request")

	resp, err := sendRequest(req, kcsTimeout)
	check.Panic(err, log, "Failed to send request")

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, err := ioutil.ReadAll(resp.Body)
		check.Panic(err, log, "Failed to read response body")
		log.WithField("resp", string(b)).Panic("Response status is not OK")
	}

	var c1 StatusResponse

	err = json.NewDecoder(resp.Body).Decode(&c1)
	check.Panic(err, log, "Failed to parse json response")

	log.WithField("resp", c1).Debug("Received response from KCS")
	return c1
}
