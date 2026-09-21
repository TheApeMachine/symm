package compiler

import (
	"context"
	"fmt"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/schemas"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/calculus"
)

/*
Constructor instantiates a Cap'n Proto capability client from raw config bytes.
*/
type Constructor func(ctx context.Context, config []byte) (capnp.Client, error)

/*
Factory pairs a Cap'n Proto capability constructor with its interface type ID.
*/
type Factory struct {
	InterfaceID uint64
	New         Constructor
}

/*
Registry maps primitive operation strings to Cap'n Proto capability factories.
*/
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
	}
}

func (r *Registry) Register(op string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[op] = factory
}

func (r *Registry) Resolve(op string) (Factory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, exists := r.factories[op]
	if !exists {
		return Factory{}, errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("compiler: unknown primitive type %q", op),
			nil,
		))
	}

	return factory, nil
}

func (r *Registry) ResolveMust(op string) Factory {
	f, err := r.Resolve(op)
	if err != nil {
		panic(err)
	}
	return f
}

func (r *Registry) Has(op string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.factories[op]
	return exists
}

func (r *Registry) Operations() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ops := make([]string, 0, len(r.factories))
	for op := range r.factories {
		ops = append(ops, op)
	}
	return ops
}

var (
	defaultRegistryOnce sync.Once
	defaultRegistry     *Registry
)

/*
DefaultRegistry constructs and populates the canonical compiler registry
with auto-generated Cap'n Proto schemas and primitive constructors.
*/
func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		reg := schemas.DefaultRegistry
		RegisterGeneratedSchemas(reg)

		r := NewRegistry()
		RegisterGeneratedPrimitives(r)

		// Boundary primitives. "grid" is the boundary through which a metric
		// receives the fields it registered an interest in.
		r.Register("grid", Factory{
			InterfaceID: 0,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client{}, nil
			},
		})
		r.Register("data.Source", Factory{
			InterfaceID: 0,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client{}, nil
			},
		})
		r.Register("source", Factory{
			InterfaceID: 0,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client{}, nil
			},
		})
		r.Register("metrics", Factory{
			InterfaceID: 0,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client{}, nil
			},
		})
		r.Register("data.Sink", Factory{
			InterfaceID: 0,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client{}, nil
			},
		})
		r.Register("sink", Factory{
			InterfaceID: 0,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client{}, nil
			},
		})

		// Test sources
		r.Register("test.Float64Source", Factory{
			InterfaceID: calculus.Floor_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Floor_ServerToClient(&passthroughSource{})), nil
			},
		})

		defaultRegistry = r
	})

	return defaultRegistry
}

type passthroughSource struct {
	out float64
}

func (s *passthroughSource) Write(ctx context.Context, call calculus.Floor_write) error {
	s.out = call.Args().Value()
	return nil
}

func (s *passthroughSource) Done(ctx context.Context, call calculus.Floor_done) error {
	res, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"passthroughSource: failed to allocate done results",
			err,
		))
	}
	res.SetOut(s.out)
	s.out = 0
	return nil
}
