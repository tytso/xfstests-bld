package gcp

import (
	"os"
	"reflect"
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
	tmpFile := "/tmp/gce-xfstests-test.config"
	for _, e := range tests {
		file, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			t.Error(err)
		}
		_, err = file.WriteString(e.content)
		file.Close()
		if err != nil {
			t.Error(err)
		}

		config, err := Get(tmpFile)
		if err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(config.kv, e.config) {
			t.Errorf("get wrong config map %v", config.kv)
		}
		os.Remove(tmpFile)
	}
}
