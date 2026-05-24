package hawkes

import (
	"math"
	"time"
)

const (
	optimizeMaxIter   = 120
	optimizeStepScale = 0.05
)

/*
optimizeBivariate fits Hawkes parameters with bound-constrained coordinate descent.
*/
func optimizeBivariate(
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
	context FitContext,
	start BivariateFit,
) BivariateFit {
	if !context.enoughEvents(buyEvents, sellEvents) {
		return BivariateFit{}
	}

	params := fitParamsFromStart(start, context, buyEvents, sellEvents)
	bestLL := objective(params, buyEvents, sellEvents, horizon, context)
	bestParams := params

	for range optimizeMaxIter {
		improved := false

		for index := range len(params) {
			for _, step := range []float64{optimizeStepScale, -optimizeStepScale} {
				candidate := append([]float64(nil), bestParams...)
				candidate[index] += step
				ll := objective(candidate, buyEvents, sellEvents, horizon, context)

				if ll <= bestLL {
					continue
				}

				bestLL = ll
				bestParams = candidate
				improved = true
			}
		}

		if !improved {
			break
		}
	}

	return paramsToFit(bestParams, buyEvents, sellEvents, horizon, context)
}

func fitParamsFromStart(
	start BivariateFit,
	context FitContext,
	buyEvents, sellEvents []time.Time,
) []float64 {
	muBuy := float64(len(buyEvents)) / context.SpanSec
	muSell := float64(len(sellEvents)) / context.SpanSec
	beta := context.MedianGapSec

	if beta <= 0 {
		beta = 1
	}

	beta = 1 / beta
	branchBB := 0.2
	branchBS := 0.05
	branchSB := 0.05
	branchSS := 0.2

	if start.valid() {
		muBuy = start.MuBuy
		muSell = start.MuSell
		beta = start.Beta
		branchBB = start.AlphaBB / beta
		branchBS = start.AlphaBS / beta
		branchSB = start.AlphaSB / beta
		branchSS = start.AlphaSS / beta
	}

	return []float64{
		logPositive(muBuy),
		logPositive(muSell),
		logPositive(beta),
		logPositive(branchBB),
		logPositive(branchBS),
		logPositive(branchSB),
		logPositive(branchSS),
	}
}

func objective(
	params []float64,
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
	context FitContext,
) float64 {
	fit := paramsToFit(params, buyEvents, sellEvents, horizon, context)

	if fit.MuBuy <= 0 {
		return math.Inf(-1)
	}

	return bivariateLogLikelihood(
		buyEvents, sellEvents, horizon,
		fit.MuBuy, fit.MuSell,
		fit.AlphaBB, fit.AlphaBS, fit.AlphaSB, fit.AlphaSS,
		fit.Beta,
	)
}

func paramsToFit(
	params []float64,
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
	context FitContext,
) BivariateFit {
	if len(params) != bivariateParamCount {
		return BivariateFit{}
	}

	muBuy := math.Exp(params[0])
	muSell := math.Exp(params[1])
	beta := math.Exp(params[2])
	branchBB := math.Exp(params[3])
	branchBS := math.Exp(params[4])
	branchSB := math.Exp(params[5])
	branchSS := math.Exp(params[6])

	if branchBB > context.BranchCeiling {
		return BivariateFit{}
	}

	if branchSS > context.BranchCeiling {
		return BivariateFit{}
	}

	crossCap := crossBranchShare(
		math.Max(branchBB, branchSS),
		context.BranchCeiling,
	)

	if branchBS > crossCap || branchSB > crossCap {
		return BivariateFit{}
	}

	return evaluateBivariateFit(
		buyEvents, sellEvents, horizon, context,
		muBuy, muSell,
		branchBB*beta, branchBS*beta,
		branchSB*beta, branchSS*beta,
		beta,
	)
}

func logPositive(value float64) float64 {
	if value <= 0 {
		return math.Log(1e-9)
	}

	return math.Log(value)
}
