package compiler

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/catalog"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
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
			childClosure := fn()
			return func(in any) (out any) {
				if in == nil {
					in = 0.0
				}
				defer func() {
					if r := recover(); r != nil {
						out = 0.0
					}
				}()
				return childClosure(in)
			}, nil
		}
	}

	registry.factories["data.Extract"] = func(node Node) (types.Value[any, any], error) {
		key := "value"

		if node.InputData != nil {
			if k, ok := node.InputData["key"].(string); ok && k != "" {
				key = k
			} else if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if p, ok := cfg["path"].(string); ok && p != "" {
					key = p
				} else if k, ok := cfg["key"].(string); ok && k != "" {
					key = k
				}
			}
		}

		closure := data.NewExtract(key)

		return func(in any) any {
			return closure(in)
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

	registry.factories["transport.Pace"] = func(node Node) (types.Value[any, any], error) {
		delay := time.Duration(0)

		if node.InputData != nil {
			if d, ok := node.InputData["delay"].(float64); ok && d > 0 {
				delay = time.Duration(d) * time.Millisecond
			} else if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if d, ok := cfg["delay"].(float64); ok && d > 0 {
					delay = time.Duration(d) * time.Millisecond
				}
			}
		}

		closure := transport.NewPace[any](delay)

		return func(in any) any {
			return closure(in)
		}, nil
	}

	registry.factories["transport.Process"] = func(node Node) (types.Value[any, any], error) {
		binary := ""
		var defaultArgs []string

		if node.InputData != nil {
			if b, ok := node.InputData["binary"].(string); ok {
				binary = b
			}
			if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if b, ok := cfg["binary"].(string); ok {
					binary = b
				}
				if args, ok := cfg["args"].([]any); ok {
					for _, a := range args {
						if s, ok := a.(string); ok {
							defaultArgs = append(defaultArgs, s)
						}
					}
				}
			}
		}

		closure := transport.NewProcess(binary, defaultArgs...)

		return func(in any) any {
			var args []string
			if in != nil {
				if strList, ok := in.([]string); ok {
					args = strList
				} else if str, ok := in.(string); ok {
					args = []string{str}
				}
			}
			return closure(args)
		}, nil
	}

	registry.factories["data.Select"] = func(node Node) (types.Value[any, any], error) {
		path := ""

		if node.InputData != nil {
			if p, ok := node.InputData["path"].(string); ok {
				path = p
			} else if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if p, ok := cfg["path"].(string); ok {
					path = p
				}
			}
		}

		closure := data.NewSelect(path)

		return func(in any) any {
			return closure(in)
		}, nil
	}

	registry.factories["ui.Broadcast"] = func(node Node) (types.Value[any, any], error) {
		return func(in any) any {
			return in
		}, nil
	}

	registry.factories["temporal.Delay"] = func(node Node) (types.Value[any, any], error) {
		horizon := 1

		if node.InputData != nil {
			if h, ok := node.InputData["horizon"].(float64); ok && h > 0 {
				horizon = int(h)
			}

			if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if h, ok := cfg["horizon"].(float64); ok && h > 0 {
					horizon = int(h)
				}
			}
		}

		closure := temporal.NewDelay[float64](horizon)

		return func(in any) any {
			var val float64
			if in != nil {
				if f, ok := in.(float64); ok {
					val = f
				}
			}
			return closure(val)
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
