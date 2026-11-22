package gcp

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

var tests = []struct {
	content string
	config  map[string]string
}{
	{`declare -- VALUE_A="a"
declare -x VALUE_B="b"
declare -- VALUE_C = "c"
declare -- VALUE_D=""
invalid line
`,
		map[string]string{
			"VALUE_A": "a",
			"VALUE_B": "b",
			"VALUE_D": "",
		},
	},
	{`VALUE_A=a
VALUE_B = b
VALUE_C="c"
VALUE_D=
invalid line
`,
		map[string]string{
			"VALUE_A": "a",
			"VALUE_C": "\"c\"",
			"VALUE_D": "",
		},
	},
}

func TestGet(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "gce-xfstests-test.config")
	for _, e := range tests {
		if err := os.WriteFile(tmpFile, []byte(e.content), 0644); err != nil {
			t.Fatal(err)
		}

		config, err := Get(tmpFile)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(config.kv, e.config) {
			t.Errorf("get wrong config map %v", config.kv)
		}
	}
}

func TestUpdateConcurrentAccessAndErrorPreservation(t *testing.T) {
	origGce, origLTM, origKCS := gce, ltm, kcs
	origGceCfg, origLTMCfg, origKCSCfg := GceConfig(), LTMConfig(), KCSConfig()
	t.Cleanup(func() {
		configLock.Lock()
		gce, ltm, kcs = origGce, origLTM, origKCS
		gceConfig, ltmConfig, kcsConfig = origGceCfg, origLTMCfg, origKCSCfg
		configLock.Unlock()
	})

	dir := t.TempDir()
	gcePath := filepath.Join(dir, "gce.config")
	ltmPath := filepath.Join(dir, "ltm.config")
	kcsPath := filepath.Join(dir, "kcs.config")

	writeAtomic := func(dst, content string) {
		tmp := dst + ".tmp"
		if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, dst); err != nil {
			t.Fatal(err)
		}
	}

	writeAtomic(gcePath, "GCE_PROJECT=test-proj\nGS_BUCKET=test-bucket\n")
	writeAtomic(ltmPath, "GCE_LTM_INT_IP=10.0.0.2\n")
	writeAtomic(kcsPath, "GCE_KCS_INT_IP=10.0.0.3\n")

	configLock.Lock()
	gce, ltm, kcs = gcePath, ltmPath, kcsPath
	configLock.Unlock()

	if err := Update(); err != nil {
		t.Fatalf("initial Update() failed: %v", err)
	}

	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				if id == 0 {
					writeAtomic(kcsPath, fmt.Sprintf("GCE_KCS_INT_IP=10.0.0.%d\n", (i%50)+2))
				}
				if err := Update(); err != nil {
					t.Errorf("Update() failed: %v", err)
				}
			}
		}(w)
	}
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if proj, err := GceConfig().Get("GCE_PROJECT"); err != nil || proj != "test-proj" {
					t.Errorf("unexpected GCE_PROJECT=%q err=%v", proj, err)
				}
				if ip, err := LTMConfig().Get("GCE_LTM_INT_IP"); err != nil || ip != "10.0.0.2" {
					t.Errorf("unexpected GCE_LTM_INT_IP=%q err=%v", ip, err)
				}
				if ip, err := KCSConfig().Get("GCE_KCS_INT_IP"); err != nil || ip == "" {
					t.Errorf("unexpected GCE_KCS_INT_IP=%q err=%v", ip, err)
				}
			}
		}()
	}
	wg.Wait()

	// Point gce at a missing file and verify Update() preserves prior configs.
	configLock.Lock()
	gce = filepath.Join(dir, "missing.config")
	configLock.Unlock()

	if err := Update(); err == nil {
		t.Fatal("expected Update() to fail when gce config is missing")
	}
	if proj, err := GceConfig().Get("GCE_PROJECT"); err != nil || proj != "test-proj" {
		t.Fatalf("expected existing GceConfig to be preserved on error, got %q (err=%v)", proj, err)
	}
}
