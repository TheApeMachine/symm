package compiler

import (
	"context"
	"sync"
	"sync/atomic"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
Runtime manages an active immutable Program and safe hot recompilation.
Editing a graph compiles a candidate Program while the current Program runs.
The active Program is only swapped at a quiescent boundary after validation succeeds.
*/
type Runtime struct {
	active   atomic.Pointer[Program]
	registry *Registry
	repo     DefinitionRepository
	mu       sync.RWMutex // quiescent swap and admission barrier
}

func NewRuntime(initial *Program, reg *Registry, repo DefinitionRepository) *Runtime {
	if reg == nil {
		reg = DefaultRegistry()
	}
	r := &Runtime{
		registry: reg,
		repo:     repo,
	}
	if initial != nil {
		r.active.Store(initial)
	}
	return r
}

/*
NewRuntimeFromJSON compiles initial JSON and constructs a Runtime with the active Program.
*/
func NewRuntimeFromJSON(jsonBytes []byte, reg *Registry, repo DefinitionRepository) (*Runtime, error) {
	prog, err := CompileJSON(jsonBytes, reg, repo)
	if err != nil {
		return nil, err
	}
	return NewRuntime(prog, reg, repo), nil
}

/*
Active returns the currently active immutable Program.
*/
func (r *Runtime) Active() *Program {
	return r.active.Load()
}

/*
Execute runs an evaluation observation through the currently active program.
Acquires read lock on the admission barrier so that hot-recompilation cannot release
capabilities while an evaluation frame is in progress.
*/
func (r *Runtime) Execute(ctx context.Context, initialInputs map[NodeID]capnp.Struct) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prog := r.active.Load()
	if prog == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"runtime: no active program",
			nil,
		))
	}
	return prog.Execute(ctx, initialInputs)
}

/*
Recompile compiles a candidate Program from doc while the active Program runs.
If candidate compilation fails, the active Program remains completely untouched.
If candidate compilation succeeds, it atomically swaps in the new Program at a
quiescent boundary and retires the old program.
*/
func (r *Runtime) Recompile(ctx context.Context, doc Graph) (*Program, error) {
	prev := r.Active()
	candidate, err := CompileWithPrevious(doc, r.registry, prev, r.repo)
	if err != nil {
		return nil, err
	}

	// Quiescent activation barrier: pause admission and swap
	r.mu.Lock()
	defer r.mu.Unlock()

	oldProg := r.active.Swap(candidate)
	if oldProg != nil {
		oldProg.Release()
	}

	return candidate, nil
}

/*
RecompileJSON unmarshals graph JSON and recompiles candidate Program with quiescent swap.
*/
func (r *Runtime) RecompileJSON(ctx context.Context, jsonBytes []byte) (*Program, error) {
	doc, err := ParseGraph(jsonBytes)
	if err != nil {
		return nil, err
	}
	return r.Recompile(ctx, doc)
}
