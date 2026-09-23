package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/manifest"
)

var (
	defaultRepositoryOnce sync.Once
	defaultRepositoryInst *Repository
)

/*
Repository manages graph definition retrieval, embedded asset fallback,
and in-memory customized definitions for the compiler and workbench.
*/
type Repository struct {
	mu          sync.RWMutex
	definitions map[string][]byte
}

func NewRepository() *Repository {
	return &Repository{
		definitions: make(map[string][]byte),
	}
}

func DefaultRepository() *Repository {
	defaultRepositoryOnce.Do(func() {
		defaultRepositoryInst = NewRepository()
		SetDefaultDefinitionRepository(defaultRepositoryInst)
	})

	return defaultRepositoryInst
}

/*
List returns all available definition identifiers from embedded assets and runtime overrides.
*/
func (repo *Repository) List() ([]string, error) {
	identifiers, err := manifest.List()
	if err != nil {
		identifiers = make([]string, 0)
	}

	seen := make(map[string]bool, len(identifiers))
	for _, identifier := range identifiers {
		seen[identifier] = true
	}

	repo.mu.RLock()
	defer repo.mu.RUnlock()

	for identifier := range repo.definitions {
		if !seen[identifier] {
			identifiers = append(identifiers, identifier)
		}
	}

	return identifiers, nil
}

/*
Get retrieves the raw Flume JSON definition bytes for an identifier.
*/
func (repo *Repository) Get(identifier string) ([]byte, error) {
	repo.mu.RLock()
	data, found := repo.definitions[identifier]
	if !found {
		trimmed := strings.TrimPrefix(identifier, "definition:")
		data, found = repo.definitions[trimmed]
		if !found {
			sanitized := strings.TrimSuffix(identifier, ".json")
			sanitized = strings.ReplaceAll(sanitized, ":", "_")
			data, found = repo.definitions[sanitized]
		}
	}
	repo.mu.RUnlock()

	if found {
		return data, nil
	}

	sanitized := strings.TrimSuffix(identifier, ".json")
	sanitized = strings.ReplaceAll(sanitized, ":", "_")

	embeddedData, err := manifest.ReadFile(sanitized)
	if err == nil {
		return embeddedData, nil
	}

	diskData, err := readFromDisk(sanitized)
	if err == nil {
		return diskData, nil
	}

	return nil, errnie.Error(errnie.Err(
		errnie.NotFound,
		fmt.Sprintf("compiler: definition not found for %s", identifier),
		err,
	))
}

/*
Save validates and stores a runtime definition in memory.
*/
func (repo *Repository) Save(identifier string, rawJSON []byte) error {
	var parsed map[string]any
	if err := sonic.Unmarshal(rawJSON, &parsed); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"compiler: invalid flume json definition",
			err,
		))
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()

	repo.definitions[identifier] = rawJSON
	trimmed := strings.TrimPrefix(identifier, "definition:")
	repo.definitions[trimmed] = rawJSON
	sanitized := strings.TrimSuffix(identifier, ".json")
	sanitized = strings.ReplaceAll(sanitized, ":", "_")
	repo.definitions[sanitized] = rawJSON

	return nil
}

/*
Load retrieves and unmarshals a graph definition into a compiler.Graph.
*/
func (repo *Repository) Load(name string) (Graph, error) {
	data, err := repo.Get(name)
	if err != nil {
		return Graph{}, errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("compiler: failed to load definition %s", name),
			err,
		))
	}

	// Definitions are authored JSON too, so they pass the same edge agreement.
	graph, err := ParseGraph(data)
	if err != nil {
		return Graph{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("compiler: invalid definition %s", name),
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
		candidates = append(candidates, filepath.Join(filepath.Dir(goFile), "..", "..", "manifest", name+".json"))
	}

	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err == nil {
			return data, nil
		}
	}

	return nil, os.ErrNotExist
}
