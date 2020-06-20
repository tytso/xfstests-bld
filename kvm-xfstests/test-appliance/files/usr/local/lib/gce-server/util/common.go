package util

import (
	"errors"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
)

var EmptyEnv = map[string]string{}

func Check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func CheckRun(cmd *exec.Cmd, workDir string, env map[string]string) {
	cmd.Dir = workDir
	cmd.Env = parseEnv(env)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Fatalf("%s failed with error: %s\n", cmd.String(), err)
	}
}

func CheckOutput(cmd *exec.Cmd, workDir string, env map[string]string) string {
	cmd.Dir = workDir
	cmd.Env = parseEnv(env)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		log.Fatalf("%s failed with error: %s\n", cmd.String(), err)
	}
	return string(out)
}

func parseEnv(env map[string]string) []string {
	newEnv := os.Environ()
	for key, value := range env {
		newEnv = append(newEnv, key+"="+value)
	}
	return newEnv
}

func CreateDir(path string) {
	err := os.MkdirAll(path, 0755)
	Check(err)
}

func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func MaxIntSlice(slice []int) (int, error) {
	if len(slice) == 0 {
		return 0, errors.New("MaxIntSlice: empty slice")
	}
	max := slice[0]
	for _, i := range slice[1:] {
		max = MaxInt(max, i)
	}
	return max, nil
}

func MinIntSlice(slice []int) (int, error) {
	if len(slice) == 0 {
		return 0, errors.New("MaxIntSlice: empty slice")
	}
	max := slice[0]
	for _, i := range slice[1:] {
		max = MinInt(max, i)
	}
	return max, nil
}

// read a whole file into a slice of strings split by lines
// remove '\n' and empty lines
func ReadLines(filename string) ([]string, error) {
	lines := []string{}

	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return lines, err
	}
	lines = strings.Split(string(content), "\n")
	nonEmptyLines := lines[:0]
	for i, line := range lines {
		if line != "" {
			nonEmptyLines = append(nonEmptyLines, lines[i:i+1]...)
		}
	}
	return nonEmptyLines, nil
}
