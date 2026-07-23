package stack

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
)

/*
openRecorder creates the runtime jsonl stream under system.data_path, rotating
aside any prior file when configured so a freeze diagnosis starts on a clean
timeline.
*/
func (booter *Booter) openRecorder() (*audit.Recorder, error) {
	path, err := auditPath()

	if err != nil {
		return nil, err
	}

	if viper.GetBool("system.audit.rotate_on_boot") {
		if err := audit.Rotate(path); err != nil {
			return nil, errnie.Error(err)
		}
	}

	return audit.NewRecorder(path)
}

/*
auditPath resolves ~/.symm/data/runtime-audit.jsonl (or the configured
system.data_path equivalent) so live and test boots share one file contract.
*/
func auditPath() (string, error) {
	dataPath := strings.TrimSpace(viper.GetString("system.data_path"))

	if strings.HasPrefix(dataPath, "~/") {
		home, err := os.UserHomeDir()

		if err != nil {
			return "", errnie.Error(errnie.Err(
				errnie.IO, "failed to resolve system.data_path", err,
			))
		}

		dataPath = filepath.Join(home, strings.TrimPrefix(dataPath, "~/"))
	}

	if dataPath == "" {
		home, err := os.UserHomeDir()

		if err != nil {
			return "", errnie.Error(errnie.Err(
				errnie.IO, "failed to resolve home for audit path", err,
			))
		}

		dataPath = filepath.Join(home, ".symm", "data")
	}

	return filepath.Join(dataPath, "runtime-audit.jsonl"), nil
}
