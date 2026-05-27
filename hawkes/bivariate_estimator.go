package hawkes

import (
	"math"
	"time"
)

/*
BivariateEstimator fits joint buy/sell Hawkes parameters from an arrival stream.
*/
type BivariateEstimator struct {
	prior BivariateFit
}

/*
NewBivariateEstimator constructs an estimator with an optional warm-start prior.
*/
func NewBivariateEstimator(prior BivariateFit) *BivariateEstimator {
	return &BivariateEstimator{prior: prior}
}

/*
Fit estimates parameters via multi-start L-BFGS on the exact log-likelihood.
*/
func (estimator *BivariateEstimator) Fit(
	stream ArrivalStream,
	horizon time.Time,
) BivariateFit {
	context, ok := NewFitContext(stream, horizon)

	if !ok || !context.EnoughEvents(stream) {
		return BivariateFit{}
	}

	best := BivariateFit{}
	bestLL := math.Inf(-1)

	for _, seed := range estimator.multiStartSeeds(context, stream) {
		candidate := estimator.maximizeLikelihood(stream, horizon, context, seed)

		if candidate.MuBuy <= 0 {
			continue
		}

		if !estimator.crossLikelihoodValid(stream, horizon, candidate) {
			candidate = candidate.withCrossZeroed().WithIntensitiesAt(stream, horizon)
		}

		logLikelihood := candidate.LogLikelihood(stream, horizon)

		if !estimator.preferCandidate(best, candidate, bestLL, logLikelihood) {
			continue
		}

		bestLL = logLikelihood
		best = candidate
	}

	return best
}

func (estimator *BivariateEstimator) crossLikelihoodValid(
	stream ArrivalStream,
	horizon time.Time,
	fit BivariateFit,
) bool {
	if fit.AlphaBS <= 0 && fit.AlphaSB <= 0 {
		return true
	}

	restricted := BivariateFit{
		MuBuy:   fit.MuBuy,
		MuSell:  fit.MuSell,
		AlphaBB: fit.AlphaBB,
		AlphaSS: fit.AlphaSS,
		Beta:    fit.Beta,
	}

	return fit.LogLikelihood(stream, horizon)+1e-9 >= restricted.LogLikelihood(stream, horizon)
}

func (estimator *BivariateEstimator) preferCandidate(
	current, candidate BivariateFit,
	currentLL, candidateLL float64,
) bool {
	if candidate.MuBuy <= 0 {
		return false
	}

	if current.MuBuy <= 0 {
		return true
	}

	if candidateLL > currentLL+1e-6 {
		return true
	}

	if math.Abs(candidateLL-currentLL) > 1e-4 {
		return false
	}

	crossCurrent := current.AlphaBS + current.AlphaSB
	crossCandidate := candidate.AlphaBS + candidate.AlphaSB

	return crossCandidate > crossCurrent
}
