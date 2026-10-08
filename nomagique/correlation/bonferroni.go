package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
BonferroniInput is a p-value and the number of candidates it is tested among.
*/
type BonferroniInput struct {
	P          float64
	Candidates float64
}

/*
Bonferroni owns min(p * candidates, 1), the union bound.
*/
type Bonferroni struct {
	err error
	out float64
}

func NewBonferroni() core.Primitive {
	return &Bonferroni{}
}

func (op *Bonferroni) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*BonferroniInput)(arriving)
			val := input.P * input.Candidates

			if val > 1.0 {
				val = 1.0
			}

			op.out = val

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Bonferroni) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
