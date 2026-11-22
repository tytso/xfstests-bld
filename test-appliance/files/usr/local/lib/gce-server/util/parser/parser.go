/*
Package parser parses a gce-xfstests command line to distribute tests among
multiple shards.
*/
package parser

import (
	"encoding/base64"
	"fmt"
	"strings"
	"thunk.org/gce-server/util/check"
)

const primaryFS = "ext4"

var xfsPath = "/root"

var invalidBools = []string{
	"ltm",
	"--no-region-shard",
	"--no-email",
	"--no-junit-email",
	"--watch-skip-initial",
	"--install-kconfig",
	"--kbuild",
	"--no-action",
}
var invalidOpts = []string{
	"--instance-name",
	"--bucket-subdir",
	"--gs-bucket",
	"--email",
	"--fail-email",
	"--junit-email",
	"--gce-zone",
	"--testrunid",
	"--hooks",
	"--update-xfstests-tar",
	"--update-xfstests",
	"--update-files",
	"-n",
	"-r",
	"--machtype",
	"--kernel",
	"--modules",
	"--arch",
	"--commit",
	"--config",
	"--kconfig-opts",
	"--kbuild-opts",
	"--install-kconfig-opts",
	"--repo",
	"--watch",
	"--unwatch",
	"--bisect-good",
	"--bisect-bad",
	"--monitor-timeout",
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

// ValidateCmdLine validates that a decoded command line contains well-formed
// "-c" configuration arguments without requiring /root/fs to exist on disk.
func ValidateCmdLine(cmdLine string) error {
	if strings.ContainsAny(cmdLine, "\x00\r\n") {
		return fmt.Errorf("cmdline must not contain NUL or newline characters")
	}
	args := strings.Fields(cmdLine)
	validArgs, _ := sanitizeCmd(args)
	validArgs = expandAliases(validArgs)
	_, configArgs, err := extractConfigArgs(validArgs)
	if err != nil {
		return err
	}
	for _, configArg := range configArgs {
		for _, c := range strings.Split(configArg, ",") {
			if _, _, _, _, err := parseConfigSpec(c); err != nil {
				return err
			}
		}
	}
	return nil
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
			continue
		}
		if boolDict.Contain(arg) {
			invalidArgs = append(invalidArgs, arg)
			continue
		}
		if optDict.Contain(arg) {
			invalidArgs = append(invalidArgs, arg)
			skipIndex = true
			continue
		}
		if eqIdx := strings.IndexByte(arg, '='); eqIdx > 0 {
			optName := arg[:eqIdx]
			if optDict.Contain(optName) || boolDict.Contain(optName) {
				invalidArgs = append(invalidArgs, arg)
				continue
			}
		}
		if len(arg) > 2 && arg[0] == '-' && arg[1] != '-' && optDict.Contain(arg[:2]) {
			invalidArgs = append(invalidArgs, arg)
			continue
		}
		validArgs = append(validArgs, arg)
	}
	return validArgs, invalidArgs
}

// expandAliases expands some explicit aliases of test options.
// It only converts "smoke" to "-c 4k -g quick", since other aliases
// ("full", "quick") have no effects on -c configs.
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

func extractConfigArgs(args []string) ([]string, []string, error) {
	newArgs := make([]string, 0, len(args))
	configArgs := []string{}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-c" {
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("missing configuration argument after -c")
			}
			val := args[i+1]
			if val == "" {
				return nil, nil, fmt.Errorf("empty configuration argument after -c")
			}
			configArgs = append(configArgs, val)
			i++
			continue
		}
		if strings.HasPrefix(arg, "-c=") {
			val := strings.TrimPrefix(arg, "-c=")
			if val == "" {
				return nil, nil, fmt.Errorf("empty configuration argument in %q", arg)
			}
			configArgs = append(configArgs, val)
			continue
		}
		if len(arg) > 2 && strings.HasPrefix(arg, "-c") {
			configArgs = append(configArgs, arg[2:])
			continue
		}
		newArgs = append(newArgs, arg)
	}
	return newArgs, configArgs, nil
}

func parseConfigSpec(configArg string) (localPrimaryFS string, fs string, cfg string, hasSlash bool, err error) {
	if configArg == "" {
		return "", "", "", false, fmt.Errorf("config name must not be empty")
	}
	parts := strings.Split(configArg, "/")
	if len(parts) > 2 {
		return "", "", "", false, fmt.Errorf("invalid config %q: too many '/' segments", configArg)
	}
	if len(parts) == 1 {
		if strings.Contains(configArg, ":") {
			return "", "", "", false, fmt.Errorf("invalid config %q: unexpected ':' without '/<cfg>'", configArg)
		}
		if err := check.ValidateTestID(configArg); err != nil {
			return "", "", "", false, fmt.Errorf("invalid config %q: %w", configArg, err)
		}
		return "", "", configArg, false, nil
	}

	checkPrimary := strings.Split(parts[0], ":")
	if len(checkPrimary) > 2 {
		return "", "", "", false, fmt.Errorf("invalid config %q: too many ':' segments", configArg)
	}
	if len(checkPrimary) == 2 {
		localPrimaryFS = checkPrimary[0]
		fs = checkPrimary[1]
		if err := check.ValidateTestID(localPrimaryFS); err != nil {
			return "", "", "", false, fmt.Errorf("invalid primary fs %q: %w", localPrimaryFS, err)
		}
		if err := check.ValidateTestID(fs); err != nil {
			return "", "", "", false, fmt.Errorf("invalid fs %q: %w", fs, err)
		}
	} else {
		fs = parts[0]
		if err := check.ValidateTestID(fs); err != nil {
			return "", "", "", false, fmt.Errorf("invalid fs %q: %w", fs, err)
		}
	}
	cfg = parts[1]
	if strings.Contains(cfg, ":") {
		return "", "", "", false, fmt.Errorf("invalid cfg %q: must not contain ':'", cfg)
	}
	if err := check.ValidateTestID(cfg); err != nil {
		return "", "", "", false, fmt.Errorf("invalid cfg %q: %w", cfg, err)
	}
	return localPrimaryFS, fs, cfg, true, nil
}

// processConfigs finds the configuration args following "-c" and parses
// them. If no "-c" option is specified (or aliases like "smoke"), it uses
// primaryFS as the filesystem and "all" as the config.
func processConfigs(args []string) ([]string, map[string][]string, error) {
	configs := make(map[string][]string)
	newArgs, configArgs, err := extractConfigArgs(args)
	if err != nil {
		return []string{}, configs, err
	}

	if len(configArgs) == 0 {
		err := defaultConfigs(configs)
		if err != nil {
			return newArgs, configs, err
		}
	} else {
		for _, configArg := range configArgs {
			for _, c := range strings.Split(configArg, ",") {
				err := singleConfig(configs, c)
				if err != nil {
					return newArgs, configs, err
				}
			}
		}
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

	<primaryFS>:<fs>/<cfg> (e.g. ext4:overlay/small) - runs the <fs>/<cfg>
	config with <primaryFS> set as primary file system
*/
func singleConfig(configs map[string][]string, configArg string) error {
	localPrimaryFS, fs, cfg, hasSlash, err := parseConfigSpec(configArg)
	if err != nil {
		return err
	}

	configLines := []string{}
	if !hasSlash {
		if check.FileExists(fmt.Sprintf("%s/fs/%s", xfsPath, cfg)) {
			fs = cfg
			configLines = []string{"default"}
		} else {
			fs = primaryFS
		}
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

	if len(localPrimaryFS) > 0 {
		fs = fmt.Sprintf("%s:%s", localPrimaryFS, fs)
	}

	for _, line := range configLines {
		if !check.ContainsStr(configs[fs], line) {
			configs[fs] = append(configs[fs], line)
		}
	}
	return nil
}

// DecodeCmd decodes the base64 string in user requests.
func DecodeCmd(cmdLine string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(cmdLine)
	if err != nil {
		return "", err
	}
	if strings.IndexByte(string(data), 0) >= 0 {
		return "", fmt.Errorf("decoded cmdline must not contain NUL bytes")
	}
	trimmed := strings.TrimSpace(string(data))
	if strings.ContainsAny(trimmed, "\r\n") {
		return "", fmt.Errorf("decoded cmdline must not contain embedded newlines")
	}
	return trimmed, nil
}
