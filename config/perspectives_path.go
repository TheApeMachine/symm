package config

import "strings"

const defaultPerspectiveInstallFile = "config/perspectives.yaml"

const defaultPerspectiveRunFile = "runs/perspectives.yaml"

/*
DefaultPerspectivePath is where tune runs persist learned playbook YAML for this session.
*/
func DefaultPerspectivePath() string {
	return defaultPerspectiveRunFile
}

/*
DefaultPerspectiveInstallPath is the active registry copied from tune output and loaded at boot.
*/
func DefaultPerspectiveInstallPath() string {
	return defaultPerspectiveInstallFile
}

/*
PerspectiveLoadPath resolves the registry file for boot. When the file is
missing, configurePerspectives keeps the Go builtin playbooks in market/.
*/
func PerspectiveLoadPath() string {
	if System == nil {
		return DefaultPerspectiveInstallPath()
	}

	return PerspectiveLoadPathFor(System.PerspectiveFile)
}

func PerspectiveLoadPathFor(preferred string) string {
	path := strings.TrimSpace(preferred)

	if path == "" {
		return DefaultPerspectiveInstallPath()
	}

	return path
}
