package reward

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Primitive adapts marks to the canonical typed objective ledger. The typed
ledger owns all accounting.
*/
type Primitive struct {
	core.Base[Mark, Outcome]
	ledger Ledger
}

func New() *Primitive {
	return &Primitive{}
}

func (op *Primitive) Next(
	in iter.Seq[core.Primitive[Mark, Mark]],
) iter.Seq[core.Primitive[Outcome, Outcome]] {
	return func(yield func(core.Primitive[Outcome, Outcome]) bool) {
		for arriving := range in {
			outcome, err := op.ledger.Measure(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(outcome)) {
				return
			}
		}
	}
}
