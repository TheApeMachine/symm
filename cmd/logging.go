package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
)

func configureLogging(cmd *cobra.Command) {
	logLevel, _ := cmd.Flags().GetString("log-level")
	logDir, _ := cmd.Flags().GetString("log-dir")
	logFile, _ := cmd.Flags().GetString("log-file")
	logFileActive, _ := cmd.Flags().GetBool("log-file-active")
	logStdout, _ := cmd.Flags().GetBool("log-stdout")

	path := strings.TrimSpace(logFile)

	if path == "" {
		runID := time.Now().UTC().Format("20060102T150405Z")
		path = filepath.Join(logDir, fmt.Sprintf("symm-%s.log", runID))
	}

	cfg := &errnie.Config{
		Level: strings.TrimSpace(logLevel),
	}

	cfg.File.Active = logFileActive
	cfg.File.Path = path

	if !logStdout {
		cfg.File.Active = logFileActive
	}

	errnie.Apply(cfg)
}

func init() {
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		configureLogging(cmd)
	}
}
