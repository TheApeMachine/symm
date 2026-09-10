package hawkes

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ScoreInput sums a per-event response on one target side.
*/
type ScoreInput struct {
	Side   float64
	Scored []ScoredEvent
}

/*
Score sums a supplied per-event intensity reciprocal on one target side.
*/
type Score struct {
	core.Base[ScoreInput, float64]
}

func NewScore() *Score {
	return &Score{}
}

func (op *Score) Next(
	in iter.Seq[core.Primitive[ScoreInput, ScoreInput]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			input := arriving.Read()
			total := 0.0

			for _, event := range input.Scored {
				if event.Side != input.Side {
					continue
				}

				total += 1 / event.Intensity
			}

			if !yield(op.Carrier(total)) {
				return
			}
		}
	}
}
