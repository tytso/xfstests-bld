/*
Package parser parses a gce-xfstests command line to distribute tests among
multiple shards.
*/
package parser

import (
	"fmt"
	"gce-server/util/check"
	"sort"
	"strings"
)

const (
	primaryFS = "ext4"
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
	"--commit",
	"--repo",
	"--watch",
	"--bisect-good",
	"--bisect-bad",
}

/*
Cmd parses a cmdline into validArgs and configs.

Returns:
	validArgs - a slice of cmd args not related to test configurations.
	Parser removes arguments from the original cmd that don't make sense
	for LTM (e.g. ltm, --instance-name).

	configs - a map from filesystem names to a slice of corresponding
	configurations.  Duplicates are removed from the original cmd configs.
*/
func Cmd(cmdLine string) ([]string, map[string][]string, error) {
	args := strings.Fields(cmdLine)
	validArgs, _ := sanitizeCmd(args)
	validArgs = expandAliases(validArgs)
	return processConfigs(validArgs)
}

// sanitizeCmd removes invalid args from input cmdline.
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

// expandAliases expands some explicit aliases of test options.
// It converts "smoke" to "-c 4k -g quick" only, since other aliases
// ("full", "quick") have no affects on -c configs.
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

// processConfigs finds the configuration args following "-c" and parses
// them. If no "-c" option is specified (or aliases like "smoke"), it uses
// primaryFS as the filesystem and "all" as the config.
func processConfigs(args []string) ([]string, map[string][]string, error) {
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
		err := defaultConfigs(configs)
		if err != nil {
			return newArgs, configs, err
		}
	} else {
		for _, c := range strings.Split(configArg, ",") {
			err := singleConfig(configs, c)
			if err != nil {
				return newArgs, configs, err
			}
		}
	}

	// remove duplicates
	for key := range configs {
		tmpSet := NewSet(configs[key])
		configs[key] = tmpSet.ToSlice()
		sort.Strings(configs[key])
	}

	return newArgs, configs, nil
}

func defaultConfigs(configs map[string][]string) error {
	configFile := fmt.Sprintf("%s/fs/%s/cfg/all.list", xfsPath, primaryFS)
	lines, err := check.ReadLines(configFile)
	if err != nil {
		return err
	}

	for _, line := range lines {
		configs[primaryFS] = append(configs[primaryFS], line)
	}
	return nil
}

/*
singleConfig parses a single configuration and adds it to the map.
Possible pattern of configs:
	<fs>/<cfg> (e.g. ext4/4k) - checks /root/fs/<fs>/cfg/<cfg>.list
	for a list of configurations, and read config lines from each file.

	<fs> (e.g. ext4) - uses default config for <fs> if it exists.
	<cfg> (e.g. quick) - uses primaryFS and <cfg> as the configuration.
*/
func singleConfig(configs map[string][]string, configArg string) error {
	configLines := []string{}
	var fs, cfg string

	arg := strings.Split(configArg, "/")
	if len(arg) == 1 {
		if check.FileExists(fmt.Sprintf("%s/fs/%s", xfsPath, configArg)) {
			fs = configArg
			configLines = []string{"default"}
		} else {
			fs = primaryFS
			cfg = configArg
		}
	} else {
		fs = arg[0]
		cfg = arg[1]
	}

	if len(configLines) == 0 {
		configFile := fmt.Sprintf("%s/fs/%s/cfg/%s.list", xfsPath, fs, cfg)

		if check.FileExists(configFile) {
			lines, err := check.ReadLines(configFile)
			if err != nil {
				return err
			}
			configLines = lines
		} else {
			configFile = configFile[:len(configFile)-5]

			if check.FileExists(configFile) {
				configLines = []string{cfg}
			} else {
				return nil
			}
		}
	}

	if _, ok := configs[fs]; ok {
		configs[fs] = append(configs[fs], configLines...)
	} else {
		configs[fs] = configLines
	}
	return nil
}
