package util

import (
	"bufio"
	"os"
	"regexp"
)

const (
	gceStateDir   = "/var/lib/gce-xfstests/"
	gceConfigFile = "/usr/local/lib/gce_xfstests.config"
)

func GetConfig() map[string]string {
	config := make(map[string]string)
	file, err := os.Open(gceConfigFile)
	Check(err)
	defer file.Close()

	re := regexp.MustCompile(`declare -- (.*?)="(.*?)"`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		tokens := re.FindStringSubmatch(line)
		if len(tokens) == 3 {
			config[tokens[1]] = tokens[2]
		}
	}

	Check(scanner.Err())

	return config
}
