package gcp

import (
	"fmt"
	"regexp"
	"sync"
	"testing"

	"thunk.org/gce-server/util/check"
)

// config file locations on multiple machines
var gce = "/usr/local/lib/gce_xfstests.config"

// Config stores the parsed config key value pairs.
type Config struct {
	kv map[string]string
}

// Global config structs initialized at boot time and refreshed via Update().
// Access them through GceConfig(), LTMConfig(), and KCSConfig().
var (
	gceConfig  *Config
	ltmConfig  *Config
	kcsConfig  *Config
	configLock sync.RWMutex
	ltm        string
	kcs        string
)

func init() {
	configLock.Lock()
	defer configLock.Unlock()

	if testing.Testing() && !check.FileExists(gce) {
		gceConfig = &Config{kv: make(map[string]string)}
		return
	}

	var err error
	gceConfig, err = Get(gce)
	if err != nil {
		panic("failed to parse gce config file")
	}

	var projID = gceConfig.kv["GCE_PROJECT"]
	if projID == "" {
		panic("failed to get GCE_PROJECT from gce config")
	}

	ltm = "/root/xfstests_bld/run-fstests/.ltm_instance_" + projID
	kcs = "/root/xfstests_bld/run-fstests/.kcs_instance_" + projID

	if check.FileExists(ltm) {
		ltmConfig, err = Get(ltm)
		if err != nil {
			panic("failed to parse LTM config file")
		}
	}

	if check.FileExists(kcs) {
		kcsConfig, err = Get(kcs)
		if err != nil {
			panic("failed to parse KCS config file")
		}
	}
}

// GceConfig returns the current GCE configuration snapshot.
func GceConfig() *Config {
	configLock.RLock()
	defer configLock.RUnlock()
	return gceConfig
}

// LTMConfig returns the current LTM instance configuration snapshot.
func LTMConfig() *Config {
	configLock.RLock()
	defer configLock.RUnlock()
	return ltmConfig
}

// KCSConfig returns the current KCS instance configuration snapshot.
func KCSConfig() *Config {
	configLock.RLock()
	defer configLock.RUnlock()
	return kcsConfig
}

// Update reads three config files to generate new config kv pairs.
// It should be called after executing launch-ltm/launch-kcs.
// Existing configurations are preserved if reading any file fails.
func Update() error {
	configLock.Lock()
	defer configLock.Unlock()

	newGce, err := Get(gce)
	if err != nil {
		return fmt.Errorf("failed to parse gce config file: %w", err)
	}

	var newLTM *Config
	if ltm != "" && check.FileExists(ltm) {
		newLTM, err = Get(ltm)
		if err != nil {
			return fmt.Errorf("failed to parse LTM config file: %w", err)
		}
	}

	var newKCS *Config
	if kcs != "" && check.FileExists(kcs) {
		newKCS, err = Get(kcs)
		if err != nil {
			return fmt.Errorf("failed to parse KCS config file: %w", err)
		}
	}

	gceConfig = newGce
	ltmConfig = newLTM
	kcsConfig = newKCS
	return nil
}

// Get reads from the config file and returns a struct Config.
// It attempts to match each line with two possible config patterns.
func Get(configFile string) (*Config, error) {
	c := Config{make(map[string]string)}
	re := regexp.MustCompile(`(?:(^declare (?:--|-x) (?P<key>\S+)="(?P<value>\S*)"$)|(^(?P<key>\S+)=(?P<value>\S*))$)`)

	lines, err := check.ReadLines(configFile)
	if err != nil {
		return nil, err
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
// Return empty value or error if key is not present in config.
func (c *Config) Get(key string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("config is nil")
	}
	if val, ok := c.kv[key]; ok {
		return val, nil
	}
	return "", fmt.Errorf("%s not found in config file", key)
}
