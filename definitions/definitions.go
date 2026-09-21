package definitions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/signal"
)

type Repository struct{}

/*
Default returns the standard definition repository.
*/
func Default() compiler.DefinitionRepository {
	return &Repository{}
}

func (r *Repository) Load(name string) (compiler.Graph, error) {
	return Load(name)
}

/*
Load retrieves a JSON graph definition by name and unmarshals it into a compiler.Graph.
It looks up definitions via embedded assets first, then falls back to disk locations.
*/
func Load(name string) (compiler.Graph, error) {
	sanitized := strings.TrimSuffix(name, ".json")
	sanitized = strings.ReplaceAll(sanitized, ":", "_")

	data, err := signal.GetDefinition(sanitized)
	if err != nil {
		data, err = readFromDisk(sanitized)
	}

	if err != nil {
		return compiler.Graph{}, errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("definitions: definition not found for %s", name),
			err,
		))
	}

	var graph compiler.Graph
	if err := json.Unmarshal(data, &graph); err != nil {
		return compiler.Graph{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("definitions: invalid json for %s", name),
			err,
		))
	}

	return graph, nil
}

func readFromDisk(name string) ([]byte, error) {
	candidates := []string{
		filepath.Join("manifest", name+".json"),
		filepath.Join("..", "manifest", name+".json"),
		filepath.Join("..", "..", "manifest", name+".json"),
	}

	_, goFile, _, ok := goruntime.Caller(0)
	if ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(goFile), "..", "manifest", name+".json"))
	}

	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err == nil {
			return data, nil
		}
	}

	return nil, os.ErrNotExist
}
