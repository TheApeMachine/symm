package adaptive

import (
	"errors"
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
PathRetention owns the configured mean-shift policy for accepted observations.
*/
type PathRetention struct {
	err    error
	window core.Primitive
	out    []temporal.Price
}

func NewPathRetention(window core.Primitive) core.Primitive {
	return &PathRetention{window: window}
}

func (op *PathRetention) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			observations := *(*[]temporal.Price)(arriving)

			if len(observations) == 0 {
				op.err = errors.Join(op.err, core.ErrShape)
				return
			}

			lastVal := observations[len(observations)-1].Value
			var capacity float64

			for wPtr := range op.window.Next(transport.NewOne(unsafe.Pointer(&lastVal)).Next(nil)) {
				w := *(*WindowReading)(wPtr)
				capacity = w.Capacity
			}

			start := max(0, len(observations)-int(capacity))

			if start == 0 {
				op.out = observations
			} else {
				op.out = slices.Clone(observations[start:])
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *PathRetention) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	if op.window != nil {
		if err := op.window.Error(); err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
