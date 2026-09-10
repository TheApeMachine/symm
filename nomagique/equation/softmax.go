package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Softmax owns the shifted exponential normalization of one run of logits.
Non-finite logits fail explicitly rather than producing plausible certainty.
*/
type Softmax[U core.Floating] struct {
	core.Base[U, U]
}

func NewSoftmax[U core.Floating]() *Softmax[U] {
	return &Softmax[U]{}
}

func (op *Softmax[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		var logits []U
		finite := logic.NewFinite[U]()

		for arriving := range in {
			defined := true

			for decision := range finite.Next(transport.One(arriving)) {
				defined = decision.Read()
			}

			if !defined {
				op.Error(core.ErrShape)
				return
			}

			logits = append(logits, arriving.Read())
		}

		if len(logits) == 0 {
			op.Error(core.ErrNotHeld)
			return
		}

		shift := logits[0]

		for _, logit := range logits[1:] {
			if logit > shift {
				shift = logit
			}
		}

		var total U
		shifted := make([]U, len(logits))

		for index, logit := range logits {
			shifted[index] = U(math.Exp(float64(logit - shift)))
			total += shifted[index]
		}

		for _, value := range shifted {
			if !yield(op.Carrier(value / total)) {
				return
			}
		}
	}
}
