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

/*
compileDefinitionCapabilities preserves explicitly wired definition.self nodes
as callable Cap'n Proto programs. Ordinary value-wired definitions still expand.
A definition cannot be evaluated both by its parent and by a Consumer.
*/
func compileDefinitionCapabilities(graph Graph, registry *Registry, repository DefinitionRepository) (*Registry, error) {
	scoped := registry

	for identifier, node := range graph.Nodes {
		if !strings.HasPrefix(node.Type, "definition:") || len(node.Connections.Outputs["self"]) == 0 {
			continue
		}

		if repository == nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "compiler: callable definition requires its repository", nil))
		}

		if len(node.Connections.Inputs) != 0 || len(node.Connections.Outputs) != 1 {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "compiler: callable definition cannot also have value wires", nil))
		}

		if scoped == registry {
			scoped = NewRegistry()
			registry.mu.RLock()
			scoped.factories = maps.Clone(registry.factories)
			registry.mu.RUnlock()
		}
		definition := strings.TrimPrefix(node.Type, "definition:")
		child, err := repository.Load(definition)

		if err != nil {
			return nil, err
		}

		for port, value := range node.InputData {
			childID, field, found := strings.Cut(port, ".")
			target, exists := child.Nodes[childID]

			if !found || !exists {
				return nil, errnie.Error(errnie.Err(errnie.Validation, fmt.Sprintf("compiler: unknown definition input %q", port), nil))
			}
			target.InputData = maps.Clone(target.InputData)

			if target.InputData == nil {
				target.InputData = map[string]json.RawMessage{}
			}
			target.InputData[field] = value
			child.Nodes[childID] = target
		}
		child, err = expandDefinitions(child, repository)

		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(child)

		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Internal, "compiler: definition identity", err))
		}
		operation := fmt.Sprintf("definition-capability:%s:%x", identifier, sha256.Sum256(encoded))
		scoped.Register(operation, Factory{
			InterfaceID: runtime.State_TypeID,
			New: func(ctx context.Context, config []byte) (capnp.Client, error) {
				program, err := Compile(child, registry, repository)

				if err != nil {
					return capnp.Client{}, err
				}
				server := runtime.State_NewServer(program)
				server.NewArena = func() capnp.Arena { return capnp.MultiSegment(nil) }
				return capnp.NewClient(server), nil
			},
		})
		node.Type, node.InputData = operation, nil
		graph.Nodes[identifier] = node
	}
	return scoped, nil
}
