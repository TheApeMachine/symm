package config

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
)

const maxBasisPoints = 10000.0

func requireUnitInterval(name string, value float64) error {
	if !finite(value) || value <= 0 || value > 1 {
		return fmt.Errorf("config: %s must be in (0,1]", name)
	}

	return nil
}

func requirePositiveBasisPoints(name string, value float64) error {
	if !finite(value) || value <= 0 || value > maxBasisPoints {
		return fmt.Errorf("config: %s must be in (0,10000]", name)
	}

	return nil
}

func requirePositiveFinite(name string, value float64) error {
	if !finite(value) || value <= 0 {
		return fmt.Errorf("config: %s must be positive", name)
	}

	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func ensureParentDirCreatable(name string, filename string) error {
	if filename == "" {
		return nil
	}

	parent := filepath.Dir(filename)

	if parent == "." || parent == "" {
		return nil
	}

	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("config: %s parent directory is not creatable: %w", name, err)
	}

	return nil
}
