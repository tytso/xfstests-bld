package gcp

import (
	"fmt"
	"regexp"
	"sync"

	"gce-server/util/check"
)

// config file locations on multiple machines
const (
	gce = "/usr/local/lib/gce_xfstests.config"
	ltm = "/root/xfstests_bld/kvm-xfstests/.ltm_instance"
	kcs = "/root/xfstests_bld/kvm-xfstests/.kcs_instance"
)

// Config stores the parsed config key value pairs.
type Config struct {
	kv map[string]string
}

// Global config structs initialized at boot time.
// GceConfig should always be not nil.
var (
	GceConfig  *Config
	LtmConfig  *Config
	KcsConfig  *Config
	configLock sync.Mutex
)

func init() {
	configLock.Lock()
	defer configLock.Unlock()

	var err error
	GceConfig, err = Get(gce)
	if err != nil {
		panic("failed to parse gce config file")
	}

	if check.FileExists(ltm) {
		LtmConfig, err = Get(ltm)
		if err != nil {
			panic("failed to parse ltm config file")
		}
	}

	if check.FileExists(kcs) {
		KcsConfig, err = Get(kcs)
		if err != nil {
			panic("failed to parse kcs config file")
		}
	}
}

// Update reads three config files to generate new config kv pairs.
// It should be called after executing launch-ltm/launch-kcs.
func Update() error {
	configLock.Lock()
	defer configLock.Unlock()

	var err error
	GceConfig, err = Get(gce)
	if err != nil {
		return fmt.Errorf("failed to parse gce config file")
	}

	if check.FileExists(ltm) {
		LtmConfig, err = Get(ltm)
		if err != nil {
			return fmt.Errorf("failed to parse ltm config file")
		}
	}

	if check.FileExists(kcs) {
		KcsConfig, err = Get(kcs)
		if err != nil {
			return fmt.Errorf("failed to parse kcs config file")
		}
	}

	return nil
}

// Get reads from the config file and returns a struct Config.
// It attempts to match each line with two possible config patterns.
func Get(configFile string) (*Config, error) {
	c := Config{make(map[string]string)}
	re := regexp.MustCompile(`(?:(^declare (?:--|-x) (?P<key>\S+)="(?P<value>\S*)"$)|(^(?P<key>\S+)=(?P<value>\S*))$)`)

	lines, err := check.ReadLines(configFile)
	if err != nil {
		return &c, err
	}

	for _, line := range lines {
		tokens := re.FindStringSubmatch(line)
		if len(tokens) == 0 {
			continue
		}
		var key, value string
		for i, name := range re.SubexpNames() {
			if name == "key" && tokens[i] != "" {
				key = tokens[i]
			}
			if name == "value" && tokens[i] != "" {
				value = tokens[i]
			}
		}

		if key != "" {
			c.kv[key] = value
		}
	}

	return &c, nil
}

// Get a certain config value according to key.
// Return empty value of error if key is not present in config.
func (c *Config) Get(key string) (string, error) {
	configLock.Lock()
	defer configLock.Unlock()
	if c == nil {
		return "", fmt.Errorf("config is nil")
	}
	if val, ok := c.kv[key]; ok {
		return val, nil
	}
	return "", fmt.Errorf("%s not found in config file", key)
}
