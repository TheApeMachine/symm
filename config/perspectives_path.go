package config

import (
	"os"
	"strings"
)

const defaultPerspectiveBuiltinFile = "config/perspectives.yaml"

const defaultPerspectiveRunFile = "runs/perspectives.yaml"

/*
DefaultPerspectivePath is where tune runs persist learned playbook YAML.
*/
func DefaultPerspectivePath() string {
	return defaultPerspectiveRunFile
}

/*
PerspectiveLoadPath resolves the registry file for boot: prefer the configured
run output, then fall back to the version-controlled builtin when the run file is
missing.
*/
func PerspectiveLoadPath() string {
	if System == nil {
		return DefaultPerspectivePath()
	}

	return PerspectiveLoadPathFor(System.PerspectiveFile)
}

func PerspectiveLoadPathFor(preferred string) string {
	path := strings.TrimSpace(preferred)

	if path == "" {
		path = DefaultPerspectivePath()
	}

	if _, err := os.Stat(path); err == nil {
		return path
	}

	if path == DefaultPerspectivePath() {
		if _, err := os.Stat(defaultPerspectiveBuiltinFile); err == nil {
			return defaultPerspectiveBuiltinFile
		}
	}

	return path
}
