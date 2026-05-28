package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/phuslu/log"
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

	if config.System.LogStdoutActive {
		return
	}

	if !config.System.LogFileActive || strings.TrimSpace(path) == "" {
		return
	}

	log.DefaultLogger.Writer = &log.FileWriter{
		Filename:     path,
		EnsureFolder: true,
	}

	fmt.Fprintf(os.Stderr, "symm: logging to %s (SYMM_LOG_STDOUT=1 for console)\n", path)
}

func init() {
	configureLogging()
}
