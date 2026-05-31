package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

const engineWorkersEnv = "SYMM_ENGINE_WORKERS"

func engineWorkerCount() (int, error) {
	raw := strings.TrimSpace(os.Getenv(engineWorkersEnv))

	if raw == "" {
		return runtime.NumCPU() * 4, nil
	}

	workers, err := strconv.Atoi(raw)

	if err != nil {
		return 0, fmt.Errorf("%s: %w", engineWorkersEnv, err)
	}

	if workers <= 0 {
		return 0, fmt.Errorf("%s: must be positive", engineWorkersEnv)
	}

	return workers, nil
}
