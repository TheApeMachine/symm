package cmd

import (
	"fmt"
	"io"
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

	switch logWriterTargetFor(config.System, path) {
	case logWriterTargetStdout:
		return
	case logWriterTargetDiscard:
		log.DefaultLogger.Writer = log.IOWriter{Writer: io.Discard}

		return
	}

	log.DefaultLogger.Writer = &log.FileWriter{
		Filename:     path,
		EnsureFolder: true,
	}

	fmt.Fprintf(os.Stderr, "symm: logging to %s (SYMM_LOG_STDOUT=1 for console)\n", path)
}

type logWriterTarget int

const (
	logWriterTargetStdout logWriterTarget = iota
	logWriterTargetFile
	logWriterTargetDiscard
)

func logWriterTargetFor(cfg *config.Config, path string) logWriterTarget {
	if cfg.LogStdoutActive {
		return logWriterTargetStdout
	}

	if cfg.LogFileActive && strings.TrimSpace(path) != "" {
		return logWriterTargetFile
	}

	return logWriterTargetDiscard
}

func init() {
	configureLogging()
}
