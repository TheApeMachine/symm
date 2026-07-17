package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
)

/*
rotateCognitive renames the cognitive persist directory aside when
cognitive.reset_on_boot is set, so a bloated WAL cannot poison the next run.
*/
func rotateCognitive(persistDir string) error {
	if !viper.GetBool("cognitive.reset_on_boot") {
		return nil
	}

	info, err := os.Stat(persistDir)

	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return errnie.Err(errnie.IO, "cognitive: stat persist_dir for rotate", err)
	}

	if !info.IsDir() {
		return errnie.Err(
			errnie.Validation,
			"cognitive.persist_dir is not a directory",
			nil,
		)
	}

	stamped := audit.RotatedPath(persistDir)

	if err := os.Rename(persistDir, stamped); err != nil {
		return errnie.Err(errnie.IO, "cognitive: rotate persist_dir failed", err)
	}

	errnie.Info(fmt.Sprintf("rotated cognitive store to %s", stamped))

	return nil
}
