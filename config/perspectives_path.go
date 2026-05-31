package config

import "strings"

const defaultPerspectiveBuiltinFile = "config/perspectives.yaml"

const defaultPerspectiveRunFile = "runs/perspectives.yaml"

/*
DefaultPerspectivePath is where tune runs persist learned playbook YAML.
*/
func DefaultPerspectivePath() string {
	return defaultPerspectiveRunFile
}

/*
PerspectiveLoadPath resolves the registry file for boot. When the file is
missing, configurePerspectives keeps the Go builtin playbooks in market/.
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
		return DefaultPerspectivePath()
	}

	return path
}
