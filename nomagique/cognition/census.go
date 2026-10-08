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
		compute := func() bool {
			if op.memory == nil {
				op.Error(core.ErrShape)
				return false
			}

			root := op.memory.root.Load()

			if root == nil {
				op.Error(core.ErrShape)
				return false
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

			censusResult := &CensusResult{
				Records: float64(records),
				Span:    span,
				Classes: make(map[string]float64),
			}

			op.memory.classes.Range(func(key, value any) bool {
				name, nameOK := key.(string)
				count, countOK := value.(*atomic.Int32)

				if nameOK && countOK {
					censusResult.Classes[name] = float64(count.Load())
				}

				return true
			})

			for value := range data.NewValue(unsafe.Pointer(censusResult)).Next(nil) {
				if !yield(value) {
					return false
				}
			}

			return true
		}

		if in == nil {
			compute()
			return
		}

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if !compute() {
				return
			}
		}
	}
}
