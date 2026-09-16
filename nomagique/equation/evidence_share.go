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
	*core.PrimitiveError

	index int
	out   float64
}

func NewEvidenceShare(index int) *EvidenceShare {
	return &EvidenceShare{PrimitiveError: core.NewPrimitiveError(), index: index}
}

func (evidenceShare *EvidenceShare) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := *(*[]float64)(arriving)

			if evidenceShare.index < 0 || evidenceShare.index >= len(values) {
				evidenceShare.Error(core.ErrShape)
				return
			}

			sum := 0.0

			for _, v := range values {
				sum += v
			}

			if sum == 0 {
				evidenceShare.Error(core.ErrDomain)
				return
			}

			evidenceShare.out = values[evidenceShare.index] / sum

			if !yield(unsafe.Pointer(&evidenceShare.out)) {
				return
			}
		}
	}
}
