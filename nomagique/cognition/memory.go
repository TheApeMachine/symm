package cognition

import (
	"context"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
)

type MemoryServer struct {
	Root        *atomic.Pointer[iradix.Tree[[]byte]]
	StepCounter *atomic.Uint64
}

func NewMemory() *MemoryServer {
	return &MemoryServer{}
}

func (s *MemoryServer) Get(ctx context.Context, call Memory_get) error {
	key, err := call.Args().Key()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "failed to read key", err))
	}

	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	tree := s.Root.Load()
	if tree != nil {
		if val, ok := tree.Get(key); ok {
			_ = results.SetValue(val)
		}
	}
	return nil
}

func (s *MemoryServer) SeekPrefix(ctx context.Context, call Memory_seekPrefix) error {
	prefix, err := call.Args().Prefix()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "failed to read prefix", err))
	}

	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	tree := s.Root.Load()
	if tree == nil {
		return nil
	}

	it := tree.Root().Iterator()
	it.SeekPrefix(prefix)

	var kvs [][2][]byte
	for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
		kvs = append(kvs, [2][]byte{k, v})
	}

	list, err := results.NewPairs(int32(len(kvs)))
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc pairs", err))
	}

	for i, pair := range kvs {
		p := list.At(i)
		_ = p.SetKey(pair[0])
		_ = p.SetValue(pair[1])
	}
	return nil
}

func (s *MemoryServer) Cas(ctx context.Context, call Memory_cas) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}
	results.SetOk(true)
	return nil
}

func (s *MemoryServer) GetStep(ctx context.Context, call Memory_getStep) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	var step uint64
	if s.StepCounter != nil {
		step = s.StepCounter.Load()
	}
	results.SetStep(int64(step))
	return nil
}

func (s *MemoryServer) IncrementStep(ctx context.Context, call Memory_incrementStep) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	var step uint64
	if s.StepCounter != nil {
		step = s.StepCounter.Add(1)
	}
	results.SetStep(int64(step))
	return nil
}

func (s *MemoryServer) Done(ctx context.Context, call Memory_done) error {
	return nil
}
