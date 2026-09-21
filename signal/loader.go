package signal

import (
	"fmt"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/manifest"
)

var (
	customDefinitionsMu sync.RWMutex
	customDefinitions   = make(map[string][]byte)
)

/*
ListDefinitions returns all available signal definition IDs.
*/
func ListDefinitions() ([]string, error) {
	ids, err := manifest.List()
	if err != nil {
		ids = make([]string, 0)
	}

	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}

	customDefinitionsMu.RLock()
	defer customDefinitionsMu.RUnlock()

	for id := range customDefinitions {
		if !seen[id] {
			ids = append(ids, id)
		}
	}

	return ids, nil
}

/*
GetDefinition retrieves the raw Flume JSON definition for a signal ID.
*/
func GetDefinition(id string) ([]byte, error) {
	customDefinitionsMu.RLock()
	data, found := customDefinitions[id]
	customDefinitionsMu.RUnlock()

	if found {
		return data, nil
	}

	sanitized := strings.TrimSuffix(id, ".json")
	sanitized = strings.ReplaceAll(sanitized, ":", "_")

	embeddedData, err := manifest.ReadFile(sanitized)
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("signal: definition not found for %s", id),
			err,
		))
	}

	return embeddedData, nil
}

func SaveDefinition(id string, rawJSON []byte) error {
	var parsed map[string]any

	if err := sonic.Unmarshal(rawJSON, &parsed); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"signal: invalid flume json definition",
			err,
		))
	}

	customDefinitionsMu.Lock()
	customDefinitions[id] = rawJSON
	customDefinitionsMu.Unlock()
	return nil
}
