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
			return fmt.Errorf("config: empty path")
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
			return fmt.Errorf("config: embedded reader[%d] is nil", index)
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
		infraPath := filepath.Join(directory, "infra.yml")
		strategyPath := filepath.Join(directory, "strategy.yml")

		if _, infraErr := os.Stat(infraPath); infraErr != nil {
			continue
		}

		if _, strategyErr := os.Stat(strategyPath); strategyErr != nil {
			continue
		}

		return directory
	}

	return ""
}

func loadDefaultConfigs() error {
	directory := defaultConfigDir()

	if directory == "" {
		return loadEmbeddedConfigs()
	}

	infraPath := filepath.Join(directory, "infra.yml")
	strategyPath := filepath.Join(directory, "strategy.yml")

	return mergeConfigFiles(infraPath, strategyPath)
}

func loadEmbeddedConfigs() error {
	infraReader, infraErr := embedded.Open("cfg/infra.yml")
	strategyReader, strategyErr := embedded.Open("cfg/strategy.yml")

	if infraErr != nil {
		return fmt.Errorf("embedded infra.yml: %w", infraErr)
	}

	if strategyErr != nil {
		_ = infraReader.Close()

		return fmt.Errorf("embedded strategy.yml: %w", strategyErr)
	}

	defer infraReader.Close()
	defer strategyReader.Close()

	return mergeEmbeddedConfig(infraReader, strategyReader)
}
