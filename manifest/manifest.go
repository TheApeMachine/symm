package manifest

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed *.json
var FS embed.FS

/*
ReadFile reads a JSON definition file from the embedded manifest FS.
*/
func ReadFile(name string) ([]byte, error) {
	if !strings.HasSuffix(name, ".json") {
		name += ".json"
	}
	return FS.ReadFile(name)
}

/*
List returns all available definition IDs in the manifest directory.
*/
func List() ([]string, error) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(entry.Name(), ".json"))
	}
	return ids, nil
}
