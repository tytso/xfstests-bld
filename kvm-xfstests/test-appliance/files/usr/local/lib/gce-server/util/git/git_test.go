package git

import (
	"gce-server/util/check"
	"io/ioutil"
	"os"
	"testing"
)

var ext4Url = "https://github.com/tytso/ext4.git"

func TestNewRepository(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Error(err)
	}
	if hostname != "xfstests-kcs" {
		t.Skip("test only runs on KCS server")
	}

	repo, err := NewRepository("test", ext4Url, ioutil.Discard)
	if err != nil {
		t.Error(err)
	}
	if !check.DirExists(RefRepoDir) {
		t.Error("reference linux repo not found")
	}
	if !check.DirExists(repo.dir) {
		t.Error("repo dir not found")
	}
	err = repo.Delete()
	if err != nil {
		t.Error(err)
	}
	if check.DirExists(repo.dir) {
		t.Error("repo dir not deleted")
	}
}

func TestCheckout(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Error(err)
	}
	if hostname != "xfstests-kcs" {
		t.Skip("test only runs on KCS server")
	}

	commit := "v5.6"
	hash := "7111951b8d4973bda27ff663f2cf18b663d15b48"

	repo, err := NewRepository("test", ext4Url, ioutil.Discard)
	if err != nil {
		t.Error(err)
	}
	err = repo.Checkout(commit, ioutil.Discard)
	if err != nil {
		t.Error(err)
	}
	head, err := repo.GetCommit(ioutil.Discard)
	if err != nil {
		t.Error(err)
	}
	if head != hash {
		t.Errorf("get wrong repo head %s instead of %s", head, hash)
	}
	err = repo.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestNewRemoteRepository(t *testing.T) {
	hash := "868b66c4a7670ed3c1795cb471974342a369b1e1"

	repo, err := NewRemoteRepository(ext4Url, "bisect-test-generic-307")
	if err != nil {
		t.Error(err)
	}
	if repo.Head() != hash {
		t.Errorf("get wrong repo head %s instead of %s", repo.Head(), hash)
	}
}

func TestParseURL(t *testing.T) {
	tests := []struct {
		url string
		dir string
	}{
		{"https://github.com/XiaoyangShen/spinner_test.git",
			"github.com-XiaoyangShen-spinner_test.git-381e3d6d"},
		{"git://git.kernel.org/pub/scm/linux/kernel/git/elder/linux.git",
			"git.kernel.org-elder-linux.git-7353b8c3"},
	}
	for _, e := range tests {
		dir, err := ParseURL(e.url)
		if err != nil {
			t.Error(err)
		}
		if dir != e.dir {
			t.Errorf("get wrong parsed url %s instead of %s", dir, e.dir)
		}
	}
}
