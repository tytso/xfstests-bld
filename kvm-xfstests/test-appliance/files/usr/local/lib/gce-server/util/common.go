package util

import (
	"log"
	"os"
	"os/exec"
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
