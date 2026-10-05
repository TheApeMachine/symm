package cognition

import (
	"encoding/binary"
	"iter"
	"sync/atomic"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Census publishes how many distinct basin keys each class has introduced,
plus the record count and the longest stored token span.
*/
type Census struct {
	*core.PrimitiveError
	memory *Associate
}

func NewCensus(memory *Associate) *Census {
	return &Census{
		PrimitiveError: core.NewPrimitiveError(),
		memory:         memory,
	}
}

func (op *Census) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			records := root.Len()
			span := 0.0

			if _, held := root.Get([]byte{0}); held {
				records--
			}

			raw, held := root.Get([]byte{1})

			if held {
				records--
			}

			if held && len(raw) == 8 {
				span = float64(binary.BigEndian.Uint64(raw))
			}

			published := data.NewOutputMap()
			published.Values["records"] = float64(records)
			published.Values["span"] = span
			op.memory.classes.Range(func(key, value any) bool {
				name, nameOK := key.(string)
				count, countOK := value.(*atomic.Int32)

				if nameOK && countOK {
					published.Values[name] = float64(count.Load())
				}

				return true
			})

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
