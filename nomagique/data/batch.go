package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Batch partitions an inbound sequence of arrivals into windows of width 'size'
advancing by 'stride'. It yields each batch as a data.Value primitive holding
that batch's items.
*/
type Batch struct {
	*core.PrimitiveError
	Size       int
	Stride     int
	Primitives []core.Primitive
}

func NewBatch(size int, stride int, primitives ...core.Primitive) core.Primitive {
	return &Batch{
		PrimitiveError: core.NewPrimitiveError(),
		Size:           size,
		Stride:         stride,
		Primitives:     primitives,
	}
}

func (op *Batch) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Size <= 0 || op.Stride <= 0 {
			op.Error(core.ErrShape)
			return
		}

		source := in
		for _, primitive := range op.Primitives {
			if primitive != nil {
				source = primitive.Next(source)
			}
		}

		var items []unsafe.Pointer

		for arriving := range source {
			items = append(items, arriving)
		}

		if len(op.Primitives) > 0 {
			for i := 0; i+op.Size <= len(items); i += op.Stride {
				for _, item := range items[i : i+op.Size] {
					if !yield(item) {
						return
					}
				}
			}
			return
		}

		for i := 0; i+op.Size <= len(items); i += op.Stride {
			chunk := NewValue(items[i : i+op.Size]...)

			if !yield(unsafe.Pointer(chunk)) {
				return
			}
		}
	}
}
