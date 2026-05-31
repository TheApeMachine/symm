package cmd

import (
	"os"
	"os/exec"
)

func execEval(executable string, env map[string]string) ([]byte, error) {
	command := exec.Command(executable, "eval")
	command.Env = os.Environ()

	for key, value := range env {
		command.Env = append(command.Env, key+"="+value)
	}

	return command.Output()
}
