package sequence

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

// Values supplies configured values as a source stage in a pipeline.
type Values[T any] struct {
	*core.PrimitiveError
	values []T
}

func NewValues[T any](values ...T) *Values[T] {
	return &Values[T]{PrimitiveError: core.NewPrimitiveError(), values: values}
}

func (values *Values[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return NewValue(values.values...)
}
