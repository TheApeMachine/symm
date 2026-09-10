package hawkes

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
GradientResult adds natural-parameter derivatives to a likelihood result.
Order is mu_x, mu_y, alpha_xx, alpha_xy, alpha_yx, alpha_yy, beta.
*/
type GradientResult struct {
	LikelihoodResult
	Gradient []float64
}

/*
Gradient owns those derivatives.
*/
type Gradient struct {
	core.Base[LikelihoodResult, GradientResult]
	score       *Score
	compensator *CompensatorDerivative
}

func NewGradient() *Gradient {
	return &Gradient{score: NewScore(), compensator: NewCompensatorDerivative()}
}

func (op *Gradient) Next(
	in iter.Seq[core.Primitive[LikelihoodResult, LikelihoodResult]],
) iter.Seq[core.Primitive[GradientResult, GradientResult]] {
	return func(yield func(core.Primitive[GradientResult, GradientResult]) bool) {
		for arriving := range in {
			like := arriving.Read()
			score := func(side float64) float64 {
				value := 0.0

				for out := range op.score.Next(transport.Values(ScoreInput{Side: side, Scored: like.Scored})) {
					value = out.Read()
				}

				return value
			}

			dMuX := score(0) - like.Span
			dMuY := score(1) - like.Span
			comp := 0.0

			for out := range op.compensator.Next(transport.Values(CompensatorDerivativeInput{
				Parameters:    like.Parameters,
				IntegralX:     like.IntegralX,
				IntegralY:     like.IntegralY,
				IntegralXBeta: like.IntegralXBeta,
				IntegralYBeta: like.IntegralYBeta,
			})) {
				comp = out.Read()
			}

			if !yield(op.Carrier(GradientResult{
				LikelihoodResult: like,
				Gradient:         []float64{dMuX, dMuY, 0, 0, 0, 0, -comp},
			})) {
				return
			}
		}
	}
}
