package cmd

import (
	"os"
	"os/exec"
	"slices"
	"strings"
)

func execEval(executable string, env map[string]string) ([]byte, error) {
	command := exec.Command(executable, "eval")
	command.Env = mergeEvalEnv(os.Environ(), env)

	return command.Output()
}

func mergeEvalEnv(base []string, overrides map[string]string) []string {
	keys := make([]string, 0, len(overrides))

	for key := range overrides {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	merged := make([]string, 0, len(base)+len(keys))

	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")

		if !ok {
			merged = append(merged, entry)

			continue
		}

		if _, overridden := overrides[key]; overridden {
			continue
		}

		merged = append(merged, entry)
	}

	for _, key := range keys {
		merged = append(merged, key+"="+overrides[key])
	}

	return merged
}
