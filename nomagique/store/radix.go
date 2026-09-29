package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"sync"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Radix owns an exact byte-keyed store. Reads never fall back to a prefix or suffix.
Tree returns an immutable snapshot; callers must not mutate its byte values.
*/
type Radix struct {
	mu   sync.RWMutex
	err  error
	held *iradix.Tree[[]byte]
}

func NewRadix(current ...*iradix.Tree[[]byte]) *Radix {
	held := iradix.New[[]byte]()

	if len(current) > 0 && current[0] != nil {
		held = current[0]
	}

	return &Radix{held: held}
}

func (op *Radix) Insert(key, value []byte) {
	op.mu.Lock()
	defer op.mu.Unlock()

	if op.held == nil {
		op.held = iradix.New[[]byte]()
	}

	op.held, _, _ = op.held.Insert(bytes.Clone(key), bytes.Clone(value))
}

func (op *Radix) Get(key []byte) ([]byte, bool) {
	op.mu.RLock()
	defer op.mu.RUnlock()

	if op.held == nil {
		return nil, false
	}

	value, found := op.held.Get(key)
	return bytes.Clone(value), found
}

func (op *Radix) Tree() *iradix.Tree[[]byte] {
	op.mu.RLock()
	defer op.mu.RUnlock()
	return op.held
}

// A checkpoint encodes keys as bytes, not UTF-8 object names. Invalid UTF-8 is
// otherwise replaced by U+FFFD, silently merging distinct region signatures.
type radixCheckpoint struct {
	Format  string       `json:"format"`
	Entries []radixEntry `json:"entries"`
}

type radixEntry struct {
	Key   []byte `json:"key"`
	Value []byte `json:"value"`
}

func (op *Radix) MarshalJSON() ([]byte, error) {
	tree := op.Tree()
	checkpoint := radixCheckpoint{Format: "symm-radix/1", Entries: []radixEntry{}}

	if tree != nil {
		iterator := tree.Root().Iterator()

		for key, value, more := iterator.Next(); more; key, value, more = iterator.Next() {
			checkpoint.Entries = append(checkpoint.Entries, radixEntry{Key: key, Value: value})
		}
	}

	return json.Marshal(checkpoint)
}

func (op *Radix) UnmarshalJSON(payload []byte) error {
	var checkpoint radixCheckpoint

	if err := json.Unmarshal(payload, &checkpoint); err != nil {
		return err
	}

	if checkpoint.Format != "symm-radix/1" || checkpoint.Entries == nil {
		return fmt.Errorf("%w: unsupported radix checkpoint; binary keys must not be decoded from JSON object names", core.ErrShape)
	}

	// Validate and rebuild privately. A malformed checkpoint leaves the current
	// live tree intact instead of exposing an empty or partially restored model.
	restored := iradix.New[[]byte]()

	for _, entry := range checkpoint.Entries {
		if _, found := restored.Get(entry.Key); found {
			return fmt.Errorf("%w: duplicate radix checkpoint key %x", core.ErrShape, entry.Key)
		}

		restored, _, _ = restored.Insert(bytes.Clone(entry.Key), bytes.Clone(entry.Value))
	}

	op.mu.Lock()
	op.held = restored
	op.mu.Unlock()
	return nil
}

func (op *Radix) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			fields := *(*map[string][]byte)(arriving)
			selector, selecting := fields["selector"]
			value, writing := fields["data"]

			if writing && !selecting {
				op.Error(core.ErrShape)
				return
			}

			if writing {
				op.Insert(selector, value)
			}

			tree := op.Tree()

			if !yield(unsafe.Pointer(&tree)) {
				return
			}
		}
	}
}

func (op *Radix) Error(errs ...error) error {
	op.mu.Lock()
	defer op.mu.Unlock()

	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
