package gcp

import (
	"regexp"
	"testing"
)

func TestGetConfig(t *testing.T) {
	config, err := Get("/usr/local/lib/gce-server/util/gcp/test1.config")
	if err != nil {
		t.Error(err)
	}
	t.Logf("%+v", config)

	config, _ = Get("/usr/local/lib/gce-server/util/gcp/test2.config")
	if err != nil {
		t.Error(err)
	}
	t.Logf("%+v", config)

}

func TestRegex(t *testing.T) {
	re := regexp.MustCompile(`(?:(^declare (?:--|-x) (?P<key>\S+)="(?P<value>\S*)"$)|(^(?P<key>\S+)=(?P<value>\S*))$)`)

	for _, line := range []string{
		"declare -- VALUE_A=\"a\"",
		"declare -x VALUE_A=\"a\"",
		"declare -x VALUE_A = \"a\"",
		"declare -x VALUE_A=",
		"VALUE_B=b",
	} {
		tokens := re.FindStringSubmatch(line)
		t.Log(line, tokens, re.SubexpNames())
		if len(tokens) == 0 {
			continue
		}
		for i, name := range re.SubexpNames() {
			if name == "key" {
				t.Log("key", tokens[i])
			}
			if name == "value" {
				t.Log("value", tokens[i])
			}
		}
	}

}
