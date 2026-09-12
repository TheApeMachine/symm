package probability

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ArgmaxResult is a winning value and the first index at which it occurred.
*/
type ArgmaxResult struct {
	Index int
	Value float64
}

/*
Argmax preserves a winning value's ordinal through comparison.
*/
type Argmax struct {
	err error
	out ArgmaxResult
}

func NewArgmax() core.Primitive {
	return &Argmax{}
}

func (op *Argmax) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var best ArgmaxResult
		seen := false
		index := 0

		for arriving := range in {
			val := *(*float64)(arriving)

			if !seen || val > best.Value {
				best = ArgmaxResult{Index: index, Value: val}
				seen = true
			}

			index++
		}

		if !seen {
			return
		}

		op.out = best
		yield(unsafe.Pointer(&op.out))
	}
}

func (op *Argmax) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
