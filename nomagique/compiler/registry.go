package compiler

import (
	"fmt"
	"strings"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/catalog"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Factory is a function that takes a declarative Node definition (including any
constructor configuration in InputData) and instantiates its executable closure.
*/
type Factory func(node Node) (types.Value[any, any], error)

/*
Registry resolves declarative node names to real nomagique constructors.
*/
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
	schemas   map[string]catalog.Schema
}

var (
	defaultRegistryOnce sync.Once
	defaultRegistryInst *Registry
)

/*
DefaultRegistry returns the singleton registry populated with all scanned nomagique primitives.
*/
func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		defaultRegistryInst = NewRegistry(nil)
	})

	return defaultRegistryInst
}

/*
NewRegistry constructs a Registry populated from scanned schemas and authoritative nomagique constructors.
*/
func NewRegistry(schemas map[string]catalog.Schema) *Registry {
	if schemas == nil {
		schemas, _ = catalog.Load()
	}

	registry := &Registry{
		factories: make(map[string]Factory),
		schemas:   schemas,
	}

	for op, legacyFactory := range DefaultPrimitiveFactories {
		fn := legacyFactory
		registry.factories[op] = func(Node) (types.Value[any, any], error) {
			return fn(), nil
		}
	}

	registry.factories["data.Extract"] = func(node Node) (types.Value[any, any], error) {
		key := "value"

		if node.InputData != nil {
			if k, ok := node.InputData["key"].(string); ok && k != "" {
				key = k
			}
		}

		closure := data.NewExtract(key)

		return func(in any) any {
			if m, ok := in.(map[string]any); ok {
				return closure(m)
			}

			return nil
		}, nil
	}

	registry.factories["transport.Collect"] = func(node Node) (types.Value[any, any], error) {
		batchSize := 10

		if node.InputData != nil {
			if b, ok := node.InputData["batchSize"].(float64); ok && b > 0 {
				batchSize = int(b)
			}
		}

		closure := transport.NewCollect[any](batchSize)

		return func(in any) any {
			return closure(in)
		}, nil
	}

	registry.factories["transport.Discard"] = func(node Node) (types.Value[any, any], error) {
		closure := transport.NewDiscard[any]()

		return func(in any) any {
			return closure(in)
		}, nil
	}

	registry.factories["pipeline.Signals"] = func(node Node) (types.Value[any, any], error) {
		signals := NewSignals()

		return func(in any) any {
			return signals(in)
		}, nil
	}

	registry.factories["pipeline.Logic"] = func(node Node) (types.Value[any, any], error) {
		logic := NewLogic()

		return func(in any) any {
			readings, ok := in.([]float64)

			if !ok {
				return nil
			}

			return logic(readings)
		}, nil
	}

	registry.factories["pipeline.Execution"] = func(node Node) (types.Value[any, any], error) {
		execution := NewExecution()

		return func(in any) any {
			if eval, ok := in.(cognition.Evaluation); ok {
				return execution(eval)
			}

			return nil
		}, nil
	}

	return registry
}

/*
Register associates an operation name with a constructor factory.
*/
func (r *Registry) Register(op string, f Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.factories[op] = f
}

/*
Resolve maps a declarative JSON node to an instantiated types.Value closure.
*/
func (r *Registry) Resolve(node Node) (types.Value[any, any], error) {
	if node.Type == "source" || node.Type == "data.Source" || node.ID == "source" || node.ID == "src" {
		return func(in any) any { return in }, nil
	}

	if node.Type == "sink" || node.Type == "data.Sink" || strings.HasPrefix(node.ID, "sink") {
		return func(in any) any { return in }, nil
	}

	r.mu.RLock()
	factory, exists := r.factories[node.Type]
	r.mu.RUnlock()

	if exists && factory != nil {
		return factory(node)
	}

	return nil, errnie.Error(errnie.Err(
		errnie.NotFound,
		fmt.Sprintf("compiler: unknown primitive type %q", node.Type),
		nil,
	))
}

/*
Primitives returns the authoritative catalog of primitive schemas for UI/workbench consumers.
*/
func Primitives() (any, error) {
	reg := DefaultRegistry()
	reg.mu.RLock()
	defer reg.mu.RUnlock()

	if len(reg.schemas) > 0 {
		return reg.schemas, nil
	}

	return catalog.Load()
}
