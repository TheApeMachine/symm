package cmd

import (
	"fmt"
	"os"
	"strings"

	decision "github.com/theapemachine/symm/market"
)

func configurePerspectives(path string) error {
	path = strings.TrimSpace(path)

	if path == "" {
		return nil
	}

	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("perspectives file %q: %w", path, err)
	}

	if err := decision.LoadPerspectiveRegistry(path); err != nil {
		return fmt.Errorf("load perspectives %q: %w", path, err)
	}

	return nil
}
