package cognition

import (
	"context"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
)

type KVPairStruct struct {
	k []byte
	v []byte
}

type MemoryServer struct {
	Root        *atomic.Pointer[iradix.Tree[[]byte]]
	StepCounter *atomic.Uint64

	DownstreamGet           func(context.Context, []byte) error
	DownstreamSeekPrefix    func(context.Context, []KVPairStruct) error
	DownstreamCas           func(context.Context, bool) error
	DownstreamGetStep       func(context.Context, uint64) error
	DownstreamIncrementStep func(context.Context) error
}

func (s *MemoryServer) Get(ctx context.Context, call Memory_get) error {
	key, _ := call.Args().Key()

	tree := s.Root.Load()
	if tree == nil {
		return nil
	}

	var result []byte
	if val, ok := tree.Get(key); ok {
		result = val
	}

	if s.DownstreamGet != nil {
		return s.DownstreamGet(ctx, result)
	}
	return nil
}

func (s *MemoryServer) SeekPrefix(ctx context.Context, call Memory_seekPrefix) error {
	prefix, _ := call.Args().Prefix()

	tree := s.Root.Load()
	if tree == nil {
		return nil
	}

	it := tree.Root().Iterator()
	it.SeekPrefix(prefix)

	var kvs []KVPairStruct
	for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
		kvs = append(kvs, KVPairStruct{k, v})
	}

	if s.DownstreamSeekPrefix != nil {
		return s.DownstreamSeekPrefix(ctx, kvs)
	}
	return nil
}

func (s *MemoryServer) Cas(ctx context.Context, call Memory_cas) error {
	updates, _ := call.Args().Updates()

	for {
		oldRoot := s.Root.Load()
		if oldRoot == nil {
			return nil
		}
		txn := oldRoot.Txn()

		for i := 0; i < updates.Len(); i++ {
			pair := updates.At(i)
			k, _ := pair.Key()
			v, _ := pair.Value()
			txn.Insert(k, v)
		}

		newRoot := txn.Commit()
		if s.Root.CompareAndSwap(oldRoot, newRoot) {
			if s.DownstreamCas != nil {
				return s.DownstreamCas(ctx, true)
			}
			return nil
		}
	}
}

func (s *MemoryServer) GetStep(ctx context.Context, call Memory_getStep) error {
	step := s.StepCounter.Load()
	if s.DownstreamGetStep != nil {
		return s.DownstreamGetStep(ctx, step)
	}
	return nil
}

func (s *MemoryServer) IncrementStep(ctx context.Context, call Memory_incrementStep) error {
	s.StepCounter.Add(1)
	if s.DownstreamIncrementStep != nil {
		return s.DownstreamIncrementStep(ctx)
	}
	return nil
}

func (s *MemoryServer) Done(ctx context.Context, call Memory_done) error {
	return nil
}
