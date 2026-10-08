package equation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
EvidenceShare selects one member after normalization.
*/
type EvidenceShare struct {
	err   error
	index int
	out   float64
}

func NewEvidenceShare(index int) core.Primitive {
	return &EvidenceShare{index: index}
}

func (op *EvidenceShare) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := *(*[]float64)(arriving)

			if op.index < 0 || op.index >= len(values) {
				op.err = core.ErrShape
				return
			}

			sum := 0.0

			for _, v := range values {
				sum += v
			}

			if sum == 0 {
				op.err = core.ErrDomain
				return
			}

			op.out = values[op.index] / sum

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *EvidenceShare) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
