package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"iter"
	"sync"
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
	held *iradix.Tree[[]byte]
	out  *iradix.Tree[[]byte]
	mu   sync.RWMutex
}

func NewRadix(current ...*iradix.Tree[[]byte]) *Radix {
	held := iradix.New[[]byte]()

	if len(current) > 0 && current[0] != nil {
		held = current[0]
	}

	return &Radix{held: held}
}

func (op *Radix) Insert(key, val []byte) {
	op.mu.Lock()
	defer op.mu.Unlock()

	if op.held == nil {
		op.held = iradix.New[[]byte]()
	}

	written, _, _ := op.held.Insert(key, bytes.Clone(val))
	op.held = written
	op.out = written
}

func (op *Radix) Get(key []byte) ([]byte, bool) {
	op.mu.RLock()
	defer op.mu.RUnlock()

	if op.held == nil {
		return nil, false
	}

	return op.held.Get(key)
}

func (op *Radix) Tree() *iradix.Tree[[]byte] {
	op.mu.RLock()
	defer op.mu.RUnlock()

	return op.held
}

func (op *Radix) MarshalJSON() ([]byte, error) {
	op.mu.RLock()
	held := op.held
	op.mu.RUnlock()

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

	op.mu.Lock()
	defer op.mu.Unlock()

	op.held = iradix.New[[]byte]()

	for key, val := range entries {
		op.held, _, _ = op.held.Insert([]byte(key), val)
	}

	return nil
}


func (op *Radix) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			op.mu.Lock()
			if op.held == nil {
				op.held = iradix.New[[]byte]()
			}

			fields := *(*map[string][]byte)(arriving)
			selector, selecting := fields["selector"]
			data, writing := fields["data"]

			if !writing {
				op.out = op.held
				ptr := unsafe.Pointer(&op.out)
				op.mu.Unlock()
				if !yield(ptr) {
					return
				}
				continue
			}

			if !selecting {
				op.Error(core.ErrShape)
				op.out = op.held
				ptr := unsafe.Pointer(&op.out)
				op.mu.Unlock()
				if !yield(ptr) {
					return
				}
				continue
			}

			written, _, _ := op.held.Insert(selector, bytes.Clone(data))
			op.held = written
			op.out = written
			ptr := unsafe.Pointer(&op.out)
			op.mu.Unlock()
			
			if !yield(ptr) {
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
