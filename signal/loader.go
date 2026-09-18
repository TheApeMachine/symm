package signal

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/store"
)

//go:embed definitions/*.json
var definitionsFS embed.FS

var (
	customDefinitionsMu sync.RWMutex
	customDefinitions   = make(map[string][]byte)
)

/*
ListDefinitions returns all available signal definition IDs.
*/
func ListDefinitions() ([]string, error) {
	ids := make([]string, 0)
	seen := make(map[string]bool)

	entries, err := fs.ReadDir(definitionsFS, "definitions")

	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}

			id := strings.TrimSuffix(entry.Name(), ".json")
			ids = append(ids, id)
			seen[id] = true
		}
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

	// Try reading from embedded FS
	filename := "definitions/" + id + ".json"

	if !strings.HasSuffix(id, ".json") {
		// Also try with colon replaced by underscore (e.g. correlation:ticker -> correlation_ticker)
		sanitized := strings.ReplaceAll(id, ":", "_")
		filename = "definitions/" + sanitized + ".json"
	}

	embeddedData, err := definitionsFS.ReadFile(filename)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("signal: definition not found for %s", id),
			err,
		))
	}

	return embeddedData, nil
}

/*
SaveDefinition updates or adds a custom signal definition at runtime.
*/
func SaveDefinition(id string, rawJSON []byte) error {
	var parsed Definition

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

/*
Load compiles and registers one signal from its definition ID.
*/
func Load(
	ctx context.Context,
	grid *store.Grid[*geometry.Coordinate],
	id string,
	symbol string,
) (*Signal, error) {
	rawJSON, err := GetDefinition(id)

	if err != nil {
		return nil, err
	}

	return Compile(ctx, grid, rawJSON, symbol)
}

/*
LoadAll compiles and registers all available signal definitions.
*/
func LoadAll(
	ctx context.Context,
	grid *store.Grid[*geometry.Coordinate],
) ([]*Signal, error) {
	ids, err := ListDefinitions()

	if err != nil {
		return nil, err
	}

	signals := make([]*Signal, 0, len(ids))

	for _, id := range ids {
		sig, err := Load(ctx, grid, id, "")

		if err != nil {
			return nil, err
		}

		signals = append(signals, sig)
	}

	return signals, nil
}
