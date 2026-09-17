package strategy

import (
	"slices"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Decision gates cognition evaluations through decisive winner checks and environment legality.
Unseen contexts and ambiguous ties yield abstention (no action emitted).
Actions illegal under the current position state are rejected downstream.
*/
type Decision struct {
	*core.PrimitiveError
	holding func() bool
	out     Action
}

func NewDecision(holding ...func() bool) *Decision {
	decision := &Decision{
		PrimitiveError: core.NewPrimitiveError(),
	}

	if len(holding) > 0 && holding[0] != nil {
		decision.holding = holding[0]
	}

	return decision
}

func (decision *Decision) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if decision.Error() != nil {
			return
		}

		for arriving := range in {
			if arriving == nil {
				continue
			}

			eval := (*cognition.Evaluation)(arriving)
			if eval.Support == 0 || len(eval.Candidates) == 0 || eval.Confidence <= 0 || eval.WinnerClass == "" {
				continue
			}

			if eval.IsTie || (eval.RunnerUp != "" && eval.Contrast == 0.0) {
				continue
			}

			isHolding := false
			if decision.holding != nil {
				isHolding = decision.holding()
			}

			legal := LegalActions(isHolding)
			candidate := Action(eval.WinnerClass)

			candidateLegal := slices.Contains(legal, candidate)

			if !candidateLegal {
				continue
			}

			decision.out = candidate
			if !yield(unsafe.Pointer(&decision.out)) {
				return
			}
		}
	}
}
