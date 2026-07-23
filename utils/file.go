package utils

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

func ResolveDataPath() string {
	dataPath := strings.TrimSpace(viper.GetViper().GetString("system.data_path"))

	if strings.HasPrefix(dataPath, "~/") {
		home, err := os.UserHomeDir()

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "failed to resolve system.data_path", err,
			))

			return ""
		}

		dataPath = filepath.Join(home, strings.TrimPrefix(dataPath, "~/"))
	}

	return dataPath
}
