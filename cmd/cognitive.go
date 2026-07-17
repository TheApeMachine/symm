package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
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

	stamped := persistDir + "." + time.Now().UTC().Format("20060102T150405Z")

	if err := os.Rename(persistDir, stamped); err != nil {
		return errnie.Err(errnie.IO, "cognitive: rotate persist_dir failed", err)
	}

	errnie.Info(fmt.Sprintf("rotated cognitive store to %s", stamped))

	return nil
}
