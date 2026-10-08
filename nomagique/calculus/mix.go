package calculus

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
MixRecord is left, right, and the weight that interpolates them.
*/
type MixRecord struct {
	Left   float64
	Weight float64
	Right  float64
}

/*
Mix owns left + weight*(right-left). Zero preserves left; one selects right.
*/
type Mix struct {
	err error
	out float64
}

func NewMix() core.Primitive {
	return &Mix{}
}

func (op *Mix) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			record := *(*MixRecord)(arriving)
			op.out = record.Left + record.Weight*(record.Right-record.Left)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Mix) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
