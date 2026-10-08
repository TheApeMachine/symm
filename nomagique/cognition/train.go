package cognition

import (
	"bytes"
	"encoding/binary"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Train records every contiguous token span of one context under one class.
*/
type Train struct {
	*core.PrimitiveError
	memory *Associate
}

func NewTrain(memory *Associate) *Train {
	return &Train{
		PrimitiveError: core.NewPrimitiveError(),
		memory:         memory,
	}
}

func (op *Train) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || op.memory == nil {
				op.Error(core.ErrShape)
				return
			}

			rec := (*TrainRecord)(arriving)

			if rec == nil {
				op.Error(core.ErrShape)
				return
			}

			context := rec.Context
			class := rec.Class

			if context == "" || class == "" {
				op.Error(core.ErrDomain)
				return
			}

			feedback := rec.Feedback
			graded := rec.Graded

			contextBytes := []byte(context)
			framed := len(contextBytes) >= 12
			var starts []int
			var ends []int
			offset := 0

			for framed && offset < len(contextBytes) {
				if offset+4 > len(contextBytes) {
					framed = false
					break
				}

				count := int(binary.BigEndian.Uint32(contextBytes[offset : offset+4]))

				if count == 0 || offset+4+count*8 > len(contextBytes) {
					framed = false
					break
				}

				starts = append(starts, offset)
				ends = append(ends, offset+4+count*8)
				offset += 4 + count*8
			}

			if framed && offset != len(contextBytes) {
				framed = false
			}

			if !framed && len(contextBytes) > 8 && len(contextBytes)%8 == 0 {
				starts, ends = nil, nil
				tokens := len(contextBytes) / 8

				for index := range tokens {
					starts = append(starts, index*8)
					ends = append(ends, (index+1)*8)
				}
			}

			if len(starts) == 0 && bytes.Contains(contextBytes, []byte{0}) {
				starts, ends = nil, nil
				start := 0

				for index, item := range contextBytes {
					if item == 0 {
						if index > start {
							starts = append(starts, start)
							ends = append(ends, index)
						}

						start = index + 1
					}
				}

				if start < len(contextBytes) {
					starts = append(starts, start)
					ends = append(ends, len(contextBytes))
				}
			}

			if len(starts) == 0 && bytes.Contains(contextBytes, []byte{'/'}) {
				starts, ends = nil, nil
				start := 0

				for index, item := range contextBytes {
					if item == '/' {
						if index > start {
							starts = append(starts, start)
							ends = append(ends, index)
						}

						start = index + 1
					}
				}

				if start < len(contextBytes) {
					starts = append(starts, start)
					ends = append(ends, len(contextBytes))
				}
			}

			if len(starts) == 0 && bytes.Contains(contextBytes, []byte{'_'}) {
				starts, ends = nil, nil
				start := 0

				for index, item := range contextBytes {
					if item == '_' {
						if index > start {
							starts = append(starts, start)
							ends = append(ends, index)
						}

						start = index + 1
					}
				}

				if start < len(contextBytes) {
					starts = append(starts, start)
					ends = append(ends, len(contextBytes))
				}
			}

			if len(starts) == 0 {
				starts = []int{0}
				ends = []int{len(contextBytes)}
			}

			var lastRecords, lastSpan float64

			for spanLen := 1; spanLen <= len(starts); spanLen++ {
				for start := 0; start+spanLen <= len(starts); start++ {
					end := start + spanLen
					subContext := string(contextBytes[starts[start]:ends[end-1]])

					record := &Record{
						Context:  subContext,
						Class:    class,
						Feedback: feedback,
						Graded:   graded,
					}

					var recordVals [2]float64
					recordIdx := 0

					for pointer := range op.memory.Next(data.NewValue(unsafe.Pointer(record)).Next(nil)) {
						if recordIdx < 2 {
							recordVals[recordIdx] = *(*float64)(pointer)
							recordIdx++
						}
					}

					if err := op.memory.Error(); err != nil {
						op.Error(err)
						return
					}

					lastRecords = recordVals[0]
					lastSpan = recordVals[1]
				}
			}

			for value := range data.NewValue(lastRecords, lastSpan).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
