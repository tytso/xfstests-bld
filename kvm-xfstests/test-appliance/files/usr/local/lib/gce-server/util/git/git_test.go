package git

import (
	"os"
	"testing"
)

func TestDelete(t *testing.T) {
	repo, err := NewRepository("test", "https://github.com/tytso/ext4.git", os.Stdout)
	if err != nil {
		t.Error(err)
	}
	err = repo.Delete()
	if err != nil {
		t.Error(err)
	}
}

func TestParseGitURL(t *testing.T) {
	urls := []string{
		"https://github.com/XiaoyangShen/spinner_test.git",
		"git@github.com:XiaoyangShen/spinner_test.git",
		"git://git.kernel.org/pub/scm/linux/kernel/git/elder/linux.git",
	}
	for _, url := range urls {
		dir, err := ParseURL(url)
		t.Log(dir, err)
	}

}
