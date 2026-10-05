package cognition

import (
	"bytes"
	"encoding/gob"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Snapshot publishes the trie as text under "model". The bytes are the clock,
the span, and every association record.
*/
type Snapshot struct {
	*core.PrimitiveError
	memory *Associate
}

func NewSnapshot(memory *Associate) *Snapshot {
	return &Snapshot{
		PrimitiveError: core.NewPrimitiveError(),
		memory:         memory,
	}
}

func (op *Snapshot) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || op.memory == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			root := op.memory.root.Load()

			if root == nil {
				op.Error(core.ErrShape)
				return
			}

			var buffer bytes.Buffer
			encoder := gob.NewEncoder(&buffer)

			if err := encoder.Encode("cognition/association/1"); err != nil {
				op.Error(err)
				return
			}

			if err := encoder.Encode(root.Len()); err != nil {
				op.Error(err)
				return
			}

			iterator := root.Root().Iterator()

			for key, value, found := iterator.Next(); found; key, value, found = iterator.Next() {
				if err := encoder.Encode(key); err != nil {
					op.Error(err)
					return
				}

				if err := encoder.Encode(value); err != nil {
					op.Error(err)
					return
				}
			}

			published := data.NewTextMap()
			published.Values["model"] = buffer.String()

			for range adapter.Next(data.NewValue(published)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
