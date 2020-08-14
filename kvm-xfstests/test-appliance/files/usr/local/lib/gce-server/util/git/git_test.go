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
