package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* stageFactory owns the immutable recipe for independent authored graph nodes. */
type stageFactory struct {
	graph      []byte
	registry   *Registry
	repository DefinitionRepository
	producer   string
	node       string
	field      string
	children   map[string]runtime.State
}

/* Create constructs one independent graph capability without activating its sources. */
func (factory *stageFactory) Create(ctx context.Context, call runtime.StageFactory_create) error {
	client, err := factory.create()

	if err != nil {
		return err
	}
	defer client.Release()
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	return errnie.Error(result.SetStage(runtime.StageNode(client).AddRef()))
}

/* create constructs the authored graph through its State capability. */
func (factory *stageFactory) create() (runtime.State, error) {
	program, err := CompileJSON(factory.graph, factory.registry, factory.repository)

	if err != nil {
		return runtime.State{}, err
	}
	server := runtime.State_NewServer(program)
	server.NewArena = func() capnp.Arena { return capnp.MultiSegment(nil) }
	return runtime.State(capnp.NewClient(server)), nil
}

/* compileFactories replaces a graph prototype with its typed creation capability. */
func compileFactories(graph Graph, registry *Registry, repository DefinitionRepository) (*Registry, error) {
	scoped := registry
	for identifier, node := range graph.Nodes {
		if !strings.HasPrefix(node.Type, "factory:") {
			continue
		}

		if repository == nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "compiler: graph factory requires a repository", nil))
		}

		child, err := repository.Load(strings.TrimPrefix(node.Type, "factory:"))

		if err != nil {
			return nil, err
		}
		child, err = expandDefinitions(child, repository)

		if err != nil {
			return nil, err
		}

		if scoped == registry {
			scoped = NewRegistry()
			registry.mu.RLock()
			scoped.factories = maps.Clone(registry.factories)
			registry.mu.RUnlock()
		}
		encoded, err := json.Marshal(child)

		if err != nil {
			return nil, errnie.Error(err)
		}
		operation := fmt.Sprintf("graph-factory:%s:%x", identifier, sha256.Sum256(encoded))
		scoped.Register(operation, Factory{InterfaceID: runtime.StageFactory_TypeID, New: func(ctx context.Context, config []byte) (capnp.Client, error) {
			server := runtime.StageFactory_NewServer(&stageFactory{
				graph: encoded, registry: registry, repository: repository,
				children: make(map[string]runtime.State),
			})
			server.NewArena = func() capnp.Arena { return capnp.MultiSegment(nil) }
			return capnp.NewClient(server), nil
		}})
		node.Type = operation
		graph.Nodes[identifier] = node
	}
	return scoped, nil
}
