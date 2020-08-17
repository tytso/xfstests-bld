package parser

import (
	"os"
	"reflect"
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
