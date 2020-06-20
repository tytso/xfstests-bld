package util

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	defaultFS = "ext4"
	xfsPath   = "/root"
)

var invalidBools = []string{"ltm", "--no-region-shard", "--no-email"}
var invalidOpts = []string{
	"--instance-name",
	"--bucket-subdir",
	"--gs-bucket",
	"--email",
	"--gce-zone",
	"--image-project",
	"--testrunid",
	"--hooks",
	"--update-xfstests-tar",
	"--update-xfstests",
	"--update-files",
	"-n",
	"-r",
	"--machtype",
	"--kernel",
}

func ParseCmd(cmdLine string) ([]string, map[string][]string) {
	args := strings.Fields(cmdLine)
	validArgs, _ := sanitizeCmd(args)
	validArgs = expandAliases(validArgs)
	validArgs, configs := processConfigs(validArgs)
	return validArgs, configs
}

func sanitizeCmd(args []string) ([]string, []string) {
	boolDict := NewSet(invalidBools)
	optDict := NewSet(invalidOpts)
	validArgs := []string{}
	invalidArgs := []string{}
	skipIndex := false

	for _, arg := range args {
		if skipIndex {
			invalidArgs = append(invalidArgs, arg)
			skipIndex = false
		} else {
			if boolDict.Contain(arg) {
				invalidArgs = append(invalidArgs, arg)
			} else if optDict.Contain(arg) {
				invalidArgs = append(invalidArgs, arg)
				skipIndex = true
			} else {
				validArgs = append(validArgs, arg)
			}
		}
	}

	return validArgs, invalidArgs
}

func expandAliases(args []string) []string {
	prefixArgs := []string{}
	expandedArgs := []string{}

	for _, arg := range args {
		if arg == "smoke" {
			if len(prefixArgs) == 0 {
				prefixArgs = append(prefixArgs, "-c", "4k", "-g", "quick")
			}
		} else {
			expandedArgs = append(expandedArgs, arg)
		}
	}

	expandedArgs = append(prefixArgs, expandedArgs...)
	return expandedArgs
}

func processConfigs(args []string) ([]string, map[string][]string) {
	newArgs := make([]string, len(args))
	copy(newArgs, args)
	configArg := ""
	configs := make(map[string][]string)

	for i, arg := range args {
		if arg == "-c" {
			configArg = args[i+1]
			newArgs = append(args[:i], args[i+2:]...)
			break
		}
	}

	if configArg == "" {
		defaultConfigs(configs)
	} else {
		for _, c := range strings.Split(configArg, ",") {
			singleConfig(configs, c)
		}
	}

	// remove duplicates
	for key := range configs {
		tmpSet := NewSet(configs[key])
		configs[key] = tmpSet.ToSlice()
		sort.Strings(configs[key])
	}

	return newArgs, configs
}

func defaultConfigs(configs map[string][]string) {
	configFile := fmt.Sprintf("%s/fs/%s/cfg/all.list", xfsPath, defaultFS)
	lines, err := ReadLines(configFile)
	Check(err)

	for _, line := range lines {
		configs[defaultFS] = append(configs[defaultFS], line)
	}
}

func singleConfig(configs map[string][]string, configArg string) {
	configLines := []string{}
	var fs, cfg string

	arg := strings.Split(configArg, "/")
	if len(arg) == 1 {
		if _, err := os.Stat(fmt.Sprintf("%s/fs/%s", xfsPath, configArg)); err == nil {
			fs = configArg
			configLines = []string{"default"}
		} else {
			fs = defaultFS
			cfg = configArg
		}
	} else {
		fs = arg[0]
		cfg = arg[1]
	}

	if len(configLines) == 0 {
		configFile := fmt.Sprintf("%s/fs/%s/cfg/%s.list", xfsPath, fs, cfg)

		if _, err := os.Stat(configFile); err == nil {
			lines, err := ReadLines(configFile)
			Check(err)
			configLines = lines
		} else {
			configFile = configFile[:len(configFile)-5]

			if _, err := os.Stat(configFile); err == nil {
				configLines = []string{cfg}
			} else {
				return
			}
		}
	}

	if _, ok := configs[fs]; ok {
		configs[fs] = append(configs[fs], configLines...)
	} else {
		configs[fs] = configLines
	}
}
