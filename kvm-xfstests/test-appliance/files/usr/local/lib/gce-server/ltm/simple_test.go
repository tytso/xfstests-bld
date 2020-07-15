package main

import (
	"bufio"
	"gce-server/server"
	"gce-server/util"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"google.golang.org/api/compute/v1"
)

var repo *util.Repository

func test1() {
	reader := bufio.NewReader(os.Stdin)
	for true {
		arg, _ := reader.ReadString('\n')

		validArg, configs, _ := util.ParseCmd(arg[:len(arg)-1])
		log.Printf("%s; %+v\n", validArg, configs)
	}
}

func test2() {
	gce, _ := util.NewGceService("xfstests-xyshen")
	info, _ := gce.GetInstanceInfo("gce-xfstests-bldsrv", "us-central1-f", "xfstests-ltm")
	log.Printf("%+v", info.Metadata)
	for _, item := range info.Metadata.Items {
		log.Printf("%+v", item)
	}

	val := "ahaah"
	newMetadata := compute.Metadata{
		Fingerprint: info.Metadata.Fingerprint,
		Items: []*compute.MetadataItems{
			{
				Key:   "shutdown_reason",
				Value: &val,
			},
		},
	}
	gce.SetMetadata("gce-xfstests-bldsrv", "us-central1-f", "xfstests-ltm", &newMetadata)
}

func test3() {
	sharder := ReadSharder("/root/mock_sharder.json")
	for _, shard := range sharder.shards {
		shard.finish()
	}
	sharder.finish()
}

func test4() {
	config, _ := util.GetConfig(util.KcsConfigFile)
	log.Printf("%+v", config)

	config, _ = util.GetConfig(util.GceConfigFile)
	log.Printf("%+v", config)
}

func test5() {
	util.SendEmail("test email", "xyshen@google.com", util.GceConfigFile)
}

func test6() {
	msg := "random msg"
	content, _ := ioutil.ReadFile("/var/log/go/go.log")
	msg = msg + "\n" + string(content)
	util.SendEmail("test", msg, "xyshen@google.com")
}

func testWatcher() {
	c := server.TaskRequest{
		Options: &server.UserOptions{
			ReportEmail: "xyshen@google.com",
			GitRepo:     "https://github.com/XiaoyangShen/spinner_test.git",
			BranchName:  "master",
		},
	}

	watcher := NewGitWatcher(c, "test")

	watcher.Run()

}

func TestParseGitURL(t *testing.T) {
	urls := []string{
		"https://github.com/XiaoyangShen/spinner_test.git",
		"git@github.com:XiaoyangShen/spinner_test.git",
		"git://git.kernel.org/pub/scm/linux/kernel/git/elder/linux.git",
	}
	for _, url := range urls {
		dir, err := util.ParseGitURL(url)
		t.Log(dir, err)
	}

}
