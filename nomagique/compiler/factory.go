package compiler

import (
	capnp "capnproto.org/go/capnp/v3"
	"context"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"maps"
	"strings"
)

/* stageFactory owns the immutable recipe for independent authored graph nodes. */
type stageFactory struct {
	graph      Graph
	registry   *Registry
	repository DefinitionRepository
}

/* Create constructs one independent graph capability without activating its sources. */
func (factory *stageFactory) Create(ctx context.Context, call runtime.StageFactory_create) error {
	program, err := Compile(factory.graph, factory.registry, factory.repository)
	if err != nil {
		return err
	}
	server := runtime.State_NewServer(program)
	server.NewArena = func() capnp.Arena { return capnp.MultiSegment(nil) }
	client := runtime.State(capnp.NewClient(server))
	defer client.Release()
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(err)
	}
	return errnie.Error(result.SetStage(runtime.StageNode(client)))
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
		if len(node.Connections.Inputs) != 0 || len(node.InputData) != 0 {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "compiler: configure the factory's authored graph directly", nil))
		}
		child, err := repository.Load(strings.TrimPrefix(node.Type, "factory:"))
		if err != nil {
			return nil, err
		}
		if scoped == registry {
			scoped = NewRegistry()
			registry.mu.RLock()
			scoped.factories = maps.Clone(registry.factories)
			registry.mu.RUnlock()
		}
		operation := "graph-factory:" + identifier
		scoped.Register(operation, Factory{InterfaceID: runtime.StageFactory_TypeID, New: func(ctx context.Context, config []byte) (capnp.Client, error) {
			return capnp.Client(runtime.StageFactory_ServerToClient(&stageFactory{child, registry, repository})), nil
		}})
		node.Type = operation
		graph.Nodes[identifier] = node
	}
	return scoped, nil
}
