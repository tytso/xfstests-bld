package parser

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var ext4cfg = []string{
	"4k",
	"1k",
	"ext3",
	"encrypt",
	"nojournal",
	"ext3conv",
	"adv",
	"dioread_nolock",
	"data_journal",
	"bigalloc",
	"bigalloc_1k",
}

var tests = []struct {
	cmdline   string
	validArgs []string
	configs   map[string][]string
}{
	{
		"ltm smoke",
		[]string{"-g", "quick"},
		map[string][]string{"ext4": {"4k"}},
	},
	{
		"ltm -c ext4/4k -g quick",
		[]string{"-g", "quick"},
		map[string][]string{"ext4": {"4k"}},
	},
	{
		"ltm quick",
		[]string{"quick"},
		map[string][]string{"ext4": ext4cfg},
	},
	{
		"ltm full",
		[]string{"full"},
		map[string][]string{"ext4": ext4cfg},
	},
	{
		"ltm -g auto",
		[]string{"-g", "auto"},
		map[string][]string{"ext4": ext4cfg},
	},
	{
		"ltm -c all",
		[]string{},
		map[string][]string{"ext4": ext4cfg},
	},
	{
		"ltm -c ext4",
		[]string{},
		map[string][]string{},
	},
	{
		"ltm -c xfs/all",
		[]string{},
		map[string][]string{"xfs": {"4k", "1k"}},
	},
	{
		"ltm -c 4k",
		[]string{},
		map[string][]string{"ext4": {"4k"}},
	},
	{
		"ltm -c ext4/4k",
		[]string{},
		map[string][]string{"ext4": {"4k"}},
	},
	{ // xfs has no "default" config so shuold be excluded.
		"ltm -c ext4/4k,xfs",
		[]string{},
		map[string][]string{"ext4": {"4k"}},
	},
	{
		"ltm -c ext4/4k,xfs/all",
		[]string{},
		map[string][]string{"ext4": {"4k"}, "xfs": {"4k", "1k"}},
	},
	{
		"ltm -c ext4/invalid_cfg,invalid_fs",
		[]string{},
		map[string][]string{},
	},
	{
		"ltm -c 4k generic/001",
		[]string{"generic/001"},
		map[string][]string{"ext4": {"4k"}},
	},
	{
		"ltm --repo test.git --commit test smoke",
		[]string{"-g", "quick"},
		map[string][]string{"ext4": {"4k"}},
	},
}

func TestParse(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Error(err)
	}
	if hostname != "xfstests-ltm" && hostname != "xfstests-kcs" {
		t.Skip("test only runs on LTM or KCS server")
	}

	for _, e := range tests {
		validArgs, configs, _ := Cmd(e.cmdline)
		if !reflect.DeepEqual(e.validArgs, validArgs) {
			t.Errorf("Unmatched validArgs for cmdline %s. Should get %s but get %s instead.",
				e.cmdline, e.validArgs, validArgs,
			)
		}

		if !reflect.DeepEqual(e.configs, configs) {
			t.Errorf("Unmatched configs for cmdline %s. Should get %s but get %s instead.",
				e.cmdline, e.configs, configs,
			)
		}
	}
}

func setupFakeXfsRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	ext4Dir := filepath.Join(root, "fs", "ext4", "cfg")
	xfsDir := filepath.Join(root, "fs", "xfs", "cfg")
	ovlDir := filepath.Join(root, "fs", "overlay", "cfg")
	for _, d := range []string{ext4Dir, xfsDir, ovlDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(ext4Dir, "all.list"), []byte(strings.Join(ext4cfg, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, c := range ext4cfg {
		if err := os.WriteFile(filepath.Join(ext4Dir, c), []byte("# cfg\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(xfsDir, "all.list"), []byte("4k\n1k\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"4k", "1k"} {
		if err := os.WriteFile(filepath.Join(xfsDir, c), []byte("# cfg\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(ovlDir, "small"), []byte("# cfg\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCmdOfflineValidationAndSanitization(t *testing.T) {
	origXfsPath := xfsPath
	xfsPath = setupFakeXfsRoot(t)
	t.Cleanup(func() {
		xfsPath = origXfsPath
	})

	for _, e := range tests {
		validArgs, configs, err := Cmd(e.cmdline)
		if err != nil {
			t.Fatalf("Cmd(%q) returned unexpected error: %v", e.cmdline, err)
		}
		if !reflect.DeepEqual(e.validArgs, validArgs) {
			t.Errorf("Unmatched validArgs for %q: want %v, got %v", e.cmdline, e.validArgs, validArgs)
		}
		if !reflect.DeepEqual(e.configs, configs) {
			t.Errorf("Unmatched configs for %q: want %v, got %v", e.cmdline, e.configs, configs)
		}
	}

	validArgs, configs, err := Cmd("ltm --hooks=/evil --kernel=/etc/passwd --modules=/etc/shadow --kbuild-opts=-j8 --watch-skip-initial -n4 -r8g -c ext4:overlay/small generic/001")
	if err != nil {
		t.Fatalf("expected primaryFS:fs/cfg and --opt=val sanitization to succeed, got: %v", err)
	}
	if !reflect.DeepEqual(validArgs, []string{"generic/001"}) {
		t.Fatalf("unexpected validArgs after stripping --opt=value flags: %v", validArgs)
	}
	if !reflect.DeepEqual(configs, map[string][]string{"ext4:overlay": {"small"}}) {
		t.Fatalf("unexpected overlay configs: %v", configs)
	}

	badCmds := []string{
		"ltm -c",
		"ltm -c=",
		"ltm -c ../../etc/passwd",
		"ltm -c=../../etc/passwd",
		"ltm -c../../etc/passwd",
		"ltm -c ext4/../../etc/passwd",
		"ltm -c ext4/cfg/extra",
		"ltm -c a:b:c/4k",
		"ltm -c ext4:4k",
		"ltm -c -bad",
		"ltm -c ext4/-bad",
		"ltm -c ext4/4k,,xfs/4k",
	}
	for _, bad := range badCmds {
		if _, _, err := Cmd(bad); err == nil {
			t.Errorf("expected Cmd(%q) to fail, got nil", bad)
		}
		if err := ValidateCmdLine(bad); err == nil {
			t.Errorf("expected ValidateCmdLine(%q) to fail, got nil", bad)
		}
	}

	// DecodeCmd checks
	encoded := base64.StdEncoding.EncodeToString([]byte("ltm smoke\n"))
	decoded, err := DecodeCmd(encoded)
	if err != nil || decoded != "ltm smoke" {
		t.Fatalf("DecodeCmd valid trailing newline failed: decoded=%q err=%v", decoded, err)
	}
	for _, raw := range []string{"ltm\x00smoke", "ltm smoke\n--evil"} {
		b64 := base64.StdEncoding.EncodeToString([]byte(raw))
		if _, err := DecodeCmd(b64); err == nil {
			t.Errorf("expected DecodeCmd(%q) to reject NUL/newline", raw)
		}
	}
}
