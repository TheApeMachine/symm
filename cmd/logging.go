package cmd

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
)

func configureLogging() {
	runID := time.Now().UTC().Format("20060102T150405Z")
	path := filepath.Join(config.System.LogDir, fmt.Sprintf("symm-%s.log", runID))

	cfg := &errnie.Config{
		Level: config.System.LogLevel,
	}
	cfg.File.Active = config.System.LogFileActive
	cfg.File.Path = path

	errnie.Apply(cfg)
}

func init() {
	configureLogging()
}
