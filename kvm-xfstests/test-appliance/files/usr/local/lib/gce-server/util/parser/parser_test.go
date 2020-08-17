package parser

import (
	"bufio"
	"os"
	"testing"
)

func TestParse(t *testing.T) {
	reader := bufio.NewReader(os.Stdin)
	for true {
		arg, _ := reader.ReadString('\n')

		validArg, configs, _ := Cmd(arg[:len(arg)-1])
		t.Logf("%s; %+v\n", validArg, configs)
	}
}
