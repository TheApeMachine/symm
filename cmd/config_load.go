package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

/*
mergeConfigFiles loads each path in order into viper (later files override).
*/
func mergeConfigFiles(paths ...string) error {
	viper.Reset()
	viper.SetConfigType("yml")

	for _, path := range paths {
		path = strings.TrimSpace(path)

		if path == "" {
			continue
		}

		file, err := os.Open(path)

		if err != nil {
			return fmt.Errorf("open %q: %w", path, err)
		}

		if err := viper.MergeConfig(file); err != nil {
			_ = file.Close()

			return fmt.Errorf("merge %q: %w", path, err)
		}

		_ = file.Close()
	}

	return nil
}

func mergeEmbeddedConfig(readers ...io.Reader) error {
	viper.Reset()
	viper.SetConfigType("yml")

	for index, reader := range readers {
		if reader == nil {
			continue
		}

		if err := viper.MergeConfig(reader); err != nil {
			return fmt.Errorf("merge embedded config[%d]: %w", index, err)
		}
	}

	return nil
}

func defaultConfigDir() string {
	paths := []string{"cmd/cfg", "."}

	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".symm"))
	}

	for _, directory := range paths {
		if _, err := os.Stat(filepath.Join(directory, "infra.yml")); err == nil {
			return directory
		}
	}

	return "cmd/cfg"
}

func loadDefaultConfigs() error {
	directory := defaultConfigDir()
	infraPath := filepath.Join(directory, "infra.yml")
	strategyPath := filepath.Join(directory, "strategy.yml")

	if _, infraErr := os.Stat(infraPath); infraErr == nil {
		if _, strategyErr := os.Stat(strategyPath); strategyErr == nil {
			return mergeConfigFiles(infraPath, strategyPath)
		}
	}

	legacyPaths := []string{
		filepath.Join(directory, "config.yml"),
		"cmd/cfg/config.yml",
		"config.yml",
	}

	for _, path := range legacyPaths {
		if _, err := os.Stat(path); err != nil {
			continue
		}

		return mergeConfigFiles(path)
	}

	infraReader, infraErr := embedded.Open("cfg/infra.yml")
	strategyReader, strategyErr := embedded.Open("cfg/strategy.yml")

	if infraErr == nil && strategyErr == nil {
		defer infraReader.Close()
		defer strategyReader.Close()

		return mergeEmbeddedConfig(infraReader, strategyReader)
	}

	configReader, err := embedded.Open("cfg/config.yml")

	if err != nil {
		return fmt.Errorf("no config files found")
	}

	defer configReader.Close()

	return mergeEmbeddedConfig(configReader)
}
