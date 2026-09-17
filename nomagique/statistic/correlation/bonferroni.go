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
	*core.PrimitiveError

	out float64
}

func NewBonferroni() *Bonferroni {
	return &Bonferroni{PrimitiveError: core.NewPrimitiveError()}
}

func (bonferroni *Bonferroni) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*BonferroniInput)(arriving)
			val := min(core.Unit, input.P*input.Candidates)

			bonferroni.out = val

			if !yield(unsafe.Pointer(&bonferroni.out)) {
				return
			}
		}
	}
}
