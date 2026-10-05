package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"iter"
	"sync/atomic"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Radix associates values with ordered keys. It never mutates its configured
source: each arrival is applied to a tree and the next tree is handed over.
*/
type Radix struct {
	err  error
	held atomic.Pointer[iradix.Tree[[]byte]]
}

func NewRadix(current ...*iradix.Tree[[]byte]) *Radix {
	held := iradix.New[[]byte]()

	if len(current) > 0 && current[0] != nil {
		held = current[0]
	}

	radix := &Radix{}
	radix.held.Store(held)
	return radix
}

func (op *Radix) Insert(key, val []byte) {
	cloned := bytes.Clone(val)

	for {
		current := op.held.Load()
		base := current

		if base == nil {
			base = iradix.New[[]byte]()
		}

		written, _, _ := base.Insert(key, cloned)

		if op.held.CompareAndSwap(current, written) {
			return
		}
	}
}

func (op *Radix) Get(key []byte) ([]byte, bool) {
	tree := op.held.Load()

	if tree == nil {
		return nil, false
	}

	return tree.Get(key)
}

func (op *Radix) Tree() *iradix.Tree[[]byte] {
	return op.held.Load()
}

func (op *Radix) MarshalJSON() ([]byte, error) {
	held := op.held.Load()
	entries := make(map[string][]byte)

	if held != nil {
		iterator := held.Root().Iterator()

		for key, val, ok := iterator.Next(); ok; key, val, ok = iterator.Next() {
			entries[string(key)] = val
		}
	}

	return json.Marshal(entries)
}

func (op *Radix) UnmarshalJSON(payload []byte) error {
	var entries map[string][]byte

	if err := json.Unmarshal(payload, &entries); err != nil {
		return err
	}

	held := iradix.New[[]byte]()

	for key, val := range entries {
		held, _, _ = held.Insert([]byte(key), val)
	}

	op.held.Store(held)
	return nil
}

func (op *Radix) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			fields := *(*map[string][]byte)(arriving)
			selector, selecting := fields["selector"]
			data, writing := fields["data"]

			if !writing {
				tree := op.held.Load()

				if tree == nil {
					tree = iradix.New[[]byte]()
					op.held.CompareAndSwap(nil, tree)
					tree = op.held.Load()
				}

				if !yield(unsafe.Pointer(&tree)) {
					return
				}

				continue
			}

			if !selecting {
				op.Error(core.ErrShape)
				tree := op.held.Load()

				if tree == nil {
					tree = iradix.New[[]byte]()
					op.held.CompareAndSwap(nil, tree)
					tree = op.held.Load()
				}

				if !yield(unsafe.Pointer(&tree)) {
					return
				}

				continue
			}

			cloned := bytes.Clone(data)
			var written *iradix.Tree[[]byte]

			for {
				current := op.held.Load()
				base := current

				if base == nil {
					base = iradix.New[[]byte]()
				}

				written, _, _ = base.Insert(selector, cloned)

				if op.held.CompareAndSwap(current, written) {
					break
				}
			}

			if !yield(unsafe.Pointer(&written)) {
				return
			}
		}
	}
}

func (op *Radix) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
