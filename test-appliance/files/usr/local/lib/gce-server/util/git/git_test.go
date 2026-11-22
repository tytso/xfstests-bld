package git

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"thunk.org/gce-server/util/check"
	"thunk.org/gce-server/util/server"
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
	hostname, err := os.Hostname()
	if err != nil {
		t.Error(err)
	}
	if hostname != "xfstests-ltm" && hostname != "xfstests-kcs" {
		t.Skip("test only runs on LTM or KCS server")
	}

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
		{"https://kernel.googlesource.com/linux.git",
			"kernel.googlesource.com-linux.git-3cff5938"},
	}
	for _, e := range tests {
		dir, err := ParseURL(e.url)
		if err != nil {
			t.Errorf("ParseURL(%q) returned unexpected error: %v", e.url, err)
		}
		if dir != e.dir {
			t.Errorf("get wrong parsed url %s instead of %s", dir, e.dir)
		}
	}

	invalid := []string{
		"",
		"https://example.com",
		"https://example.com/",
		"file:///etc/passwd",
		"--upload-pack=/bin/sh",
		"ext::sh -c evil",
		"https://-evil.com/a/b.git",
		"https://github.com/tytso/../ext4.git",
		"https://github.com/tytso//ext4.git",
		"https://github.com/tytso/ext4..bad.git",
		"https://github.com/tytso/ext4.git\n",
	}
	for _, u := range invalid {
		if dir, err := ParseURL(u); err == nil {
			t.Errorf("expected ParseURL(%q) to fail, got %q", u, dir)
		}
	}
}

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func addTestCommit(t *testing.T, dir string, idx int) string {
	t.Helper()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte(fmt.Sprintf("commit-%d\n", idx)), 0644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, dir, "add", "file.txt")
	runGitCmd(t, dir, "-c", "commit.gpgsign=false", "commit", "-m", fmt.Sprintf("commit %d", idx))
	return runGitCmd(t, dir, "rev-parse", "HEAD")
}

func createLocalGitRepo(t *testing.T, numCommits int) (string, string, []string) {
	t.Helper()
	parent := t.TempDir()
	workDir := filepath.Join(parent, "origin", "linux.git")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, workDir, "init")
	runGitCmd(t, workDir, "config", "user.name", "Test User")
	runGitCmd(t, workDir, "config", "user.email", "test@example.com")
	runGitCmd(t, workDir, "checkout", "-B", "main")

	commits := make([]string, 0, numCommits)
	for i := 0; i < numCommits; i++ {
		commits = append(commits, addTestCommit(t, workDir, i))
	}
	return "file://localhost" + workDir, workDir, commits
}

func TestRemoteRepositoryConcurrentUpdateAndHead(t *testing.T) {
	repoURL, workDir, commits := createLocalGitRepo(t, 2)

	remote, err := NewRemoteRepository(repoURL, "main")
	if err != nil {
		t.Fatalf("NewRemoteRepository failed: %v", err)
	}
	if got := remote.Head(); got != commits[len(commits)-1] {
		t.Fatalf("unexpected initial head %q, want %q", got, commits[len(commits)-1])
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if h := remote.Head(); len(h) != 40 {
						t.Errorf("unexpected head length: %q", h)
					}
				}
			}
		}()
	}

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if _, err := remote.Update(); err != nil {
					t.Errorf("Update() failed: %v", err)
				}
			}
		}()
	}

	var latest string
	for i := 2; i < 5; i++ {
		latest = addTestCommit(t, workDir, i)
		if _, err := remote.Update(); err != nil {
			t.Fatalf("Update() after new commit failed: %v", err)
		}
	}

	close(stop)
	wg.Wait()

	if _, err := remote.Update(); err != nil {
		t.Fatalf("final Update() failed: %v", err)
	}
	if got := remote.Head(); got != latest {
		t.Fatalf("final Head() = %q, want %q", got, latest)
	}
}

func TestRepositoryConcurrentCloneBisectIsolationAndFlock(t *testing.T) {
	repoURL, workDir, commits := createLocalGitRepo(t, 4)

	refSourceDir := filepath.Join(t.TempDir(), "ref.git")
	runGitCmd(t, workDir, "clone", "--bare", workDir, refSourceDir)

	origRoot := repoRootDir
	origRefURL := refRepoURL
	repoRegistryMu.Lock()
	origRegistry := repoRegistry
	origPathLocks := pathLocks
	repoRootDir = filepath.Join(t.TempDir(), "cache-repos") + "/"
	refRepoURL = "file://localhost" + refSourceDir
	repoRegistry = make(map[string]*Repository)
	pathLocks = make(map[string]*sync.Mutex)
	repoRegistryMu.Unlock()

	t.Cleanup(func() {
		repoRegistryMu.Lock()
		repoRootDir = origRoot
		refRepoURL = origRefURL
		repoRegistry = origRegistry
		pathLocks = origPathLocks
		repoRegistryMu.Unlock()
	})

	for _, reserved := range []string{"linux.reference", "bisects", RepoLockFileName, "bad.tmp"} {
		if _, err := NewRepository(reserved, repoURL, io.Discard); err == nil {
			t.Fatalf("expected NewRepository(%q) to reject reserved ID", reserved)
		}
		if _, err := NewBisectRepository(reserved, repoURL, io.Discard); err == nil {
			t.Fatalf("expected NewBisectRepository(%q) to reject reserved ID", reserved)
		}
	}

	const workers = 6
	buildRepos := make([]*Repository, workers)
	bisectRepos := make([]*Repository, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			r, err := NewRepository("shared-build", repoURL, io.Discard)
			if err != nil {
				t.Errorf("NewRepository failed: %v", err)
				return
			}
			buildRepos[idx] = r
		}(i)
		go func(idx int) {
			defer wg.Done()
			r, err := NewBisectRepository("bisect-a", repoURL, io.Discard)
			if err != nil {
				t.Errorf("NewBisectRepository failed: %v", err)
				return
			}
			bisectRepos[idx] = r
		}(i)
	}
	wg.Wait()

	for i := 1; i < workers; i++ {
		if buildRepos[i] != buildRepos[0] {
			t.Fatalf("expected canonical *Repository instance for shared-build, got distinct pointers")
		}
		if bisectRepos[i] != bisectRepos[0] {
			t.Fatalf("expected canonical *Repository instance for bisect-a, got distinct pointers")
		}
	}
	if buildRepos[0] == bisectRepos[0] {
		t.Fatal("expected build and bisect repositories to be isolated")
	}

	// Verify NewBisectRepository with id == ParseURL(repoURL) does not alias
	// the shared base repository and Delete() leaves baseRepo intact.
	baseID, err := ParseURL(repoURL)
	if err != nil {
		t.Fatal(err)
	}
	baseRepo, err := NewRepository(baseID, repoURL, io.Discard)
	if err != nil {
		t.Fatalf("NewRepository(baseID) failed: %v", err)
	}
	bisectAlias, err := NewBisectRepository(baseID, repoURL, io.Discard)
	if err != nil {
		t.Fatalf("NewBisectRepository(baseID) failed: %v", err)
	}
	if bisectAlias == baseRepo || bisectAlias.Dir() == baseRepo.Dir() {
		t.Fatalf("expected bisect repo for %q to be isolated from base repo %q", baseID, baseRepo.Dir())
	}
	if err := bisectAlias.Delete(); err != nil {
		t.Fatalf("bisectAlias.Delete() failed: %v", err)
	}
	if !check.DirExists(baseRepo.Dir()) {
		t.Fatal("deleting bisect repo unexpectedly deleted shared base repo")
	}
	if !check.DirExists(repoRootDir + "linux.reference") {
		t.Fatal("deleting bisect repo unexpectedly deleted linux.reference")
	}

	// Concurrent RLock readers (GetCommit, Valid, BisectLog) alongside write
	// operations (BisectStart, BisectStep, BisectReset, Checkout).
	bisectRepo := bisectRepos[0]
	stopReaders := make(chan struct{})
	var rwg sync.WaitGroup
	for i := 0; i < 4; i++ {
		rwg.Add(1)
		go func() {
			defer rwg.Done()
			for {
				select {
				case <-stopReaders:
					return
				default:
					_, _ = bisectRepo.GetCommit(io.Discard)
					_, _ = bisectRepo.Valid(commits[0], io.Discard)
					_, _ = bisectRepo.BisectLog(io.Discard)
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	done, err := bisectRepo.BisectStart(commits[3], []string{commits[0]}, io.Discard)
	if err != nil {
		t.Fatalf("BisectStart failed: %v", err)
	}
	for steps := 0; !done; steps++ {
		if steps > 10 {
			t.Fatal("BisectStep did not converge within 10 steps")
		}
		done, err = bisectRepo.BisectStep(server.Fail, io.Discard)
		if err != nil {
			t.Fatalf("BisectStep failed: %v", err)
		}
	}
	if err := bisectRepo.BisectReset(io.Discard); err != nil {
		t.Fatalf("BisectReset failed: %v", err)
	}
	if err := bisectRepo.Checkout(commits[1], io.Discard); err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}
	close(stopReaders)
	rwg.Wait()

	// Verify flock coordination with gce-repo-cleanup (.repo-cleanup.lock).
	unlockShared, err := acquireSharedRepoLock()
	if err != nil {
		t.Fatalf("acquireSharedRepoLock failed: %v", err)
	}
	lockFile, err := os.OpenFile(filepath.Join(repoRootDir, RepoLockFileName), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		unlockShared()
		t.Fatal(err)
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != syscall.EWOULDBLOCK {
		unlockShared()
		t.Fatalf("expected non-blocking LOCK_EX to fail with EWOULDBLOCK while shared lock is held, got %v", err)
	}
	unlockShared()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("LOCK_EX failed: %v", err)
	}
	getDone := make(chan error, 1)
	go func() {
		_, err := baseRepo.GetCommit(io.Discard)
		getDone <- err
	}()

	select {
	case err := <-getDone:
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		t.Fatalf("expected GetCommit to block while exclusive cleanup lock is held (err=%v)", err)
	case <-time.After(50 * time.Millisecond):
	}

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatalf("LOCK_UN failed: %v", err)
	}
	select {
	case err := <-getDone:
		if err != nil {
			t.Fatalf("GetCommit failed after releasing exclusive lock: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for GetCommit after releasing exclusive lock")
	}
}
