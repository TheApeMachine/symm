package integration

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

func loadRepoConfig() error {
	viper.SetConfigType("yml")

	configPath, err := repoConfigPath()

	if err != nil {
		return err
	}

	viper.SetConfigFile(configPath)

	if readErr := viper.ReadInConfig(); readErr != nil {
		return fmt.Errorf("integration: read config %s: %w", configPath, readErr)
	}

	viper.Set("system.audit.enabled", false)

	return nil
}

func repoConfigPath() (string, error) {
	cwd, err := os.Getwd()

	if err != nil {
		return "", err
	}

	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "cmd", "cfg", "config.yml")

		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}

		if dir == filepath.Dir(dir) {
			break
		}
	}

	return "", fmt.Errorf("integration: cmd/cfg/config.yml not found from %s", cwd)
}
