package hawkes

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
LikelihoodInput is the bivariate exponential Hawkes log likelihood's data.
*/
type LikelihoodInput struct {
	Parameters
	Origin  float64
	Horizon float64
	Events  []Event
}

/*
LikelihoodResult is the log likelihood and the pieces used by the gradient.
*/
type LikelihoodResult struct {
	LogLikelihood float64
	LogSum        float64
	Compensator   float64
	Span          float64
	Scored        []ScoredEvent
	IntegralX     float64
	IntegralY     float64
	IntegralXBeta float64
	IntegralYBeta float64
	Parameters
}

/*
Likelihood owns that log likelihood. Invalid parameters are explicit errors.
*/
type Likelihood struct {
	core.Base[LikelihoodInput, LikelihoodResult]
	events      *EventLikelihood
	integral    *Integral
	beta        *Integral
	compensator *Compensator
}

func NewLikelihood() *Likelihood {
	return &Likelihood{
		events:      NewEventLikelihood(),
		integral:    NewIntegralSupport(),
		beta:        NewIntegralDerivative(),
		compensator: NewCompensator(),
	}
}

func (op *Likelihood) Next(
	in iter.Seq[core.Primitive[LikelihoodInput, LikelihoodInput]],
) iter.Seq[core.Primitive[LikelihoodResult, LikelihoodResult]] {
	return func(yield func(core.Primitive[LikelihoodResult, LikelihoodResult]) bool) {
		for arriving := range in {
			input := arriving.Read()

			if input.Beta <= 0 || input.MuX <= 0 || input.MuY <= 0 || input.Horizon <= input.Origin || len(input.Events) == 0 {
				op.Error(core.ErrDomain)
				return
			}

			var scored []ScoredEvent
			logSum := 0.0

			for event := range op.events.Next(transport.Values(EventLikelihoodInput{
				Parameters: input.Parameters, Origin: input.Origin, Horizon: input.Horizon, Events: input.Events,
			})) {
				scored = append(scored, event.Read())
				logSum += event.Read().LogIntensity
			}

			integral := func(side float64, kind *Integral) float64 {
				value := 0.0

				for out := range kind.Next(transport.Values(IntegralInput{
					Parameters: input.Parameters, Side: side, Origin: input.Origin, Horizon: input.Horizon, Events: input.Events,
				})) {
					value = out.Read()
				}

				return value
			}

			ix, iy := integral(0, op.integral), integral(1, op.integral)
			ixb, iyb := integral(0, op.beta), integral(1, op.beta)
			span := input.Horizon - input.Origin
			compensator := 0.0

			for out := range op.compensator.Next(transport.Values(CompensatorInput{
				Parameters: input.Parameters, Span: span, IntegralX: ix, IntegralY: iy,
			})) {
				compensator = out.Read()
			}

			if !yield(op.Carrier(LikelihoodResult{
				LogLikelihood: logSum - compensator,
				LogSum:        logSum,
				Compensator:   compensator,
				Span:          span,
				Scored:        scored,
				IntegralX:     ix,
				IntegralY:     iy,
				IntegralXBeta: ixb,
				IntegralYBeta: iyb,
				Parameters:    input.Parameters,
			})) {
				return
			}
		}
	}
}
