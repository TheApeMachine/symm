package hawkes

import (
	"math"
	"time"
)

type fitCandidate struct {
	fit           BivariateFit
	logLikelihood float64
}

/*
fitBivariate estimates joint buy/sell Hawkes parameters via likelihood grid search.
*/
func fitBivariate(buyEvents, sellEvents []time.Time, horizon time.Time) BivariateFit {
	return fitBivariateWithPrior(buyEvents, sellEvents, horizon, BivariateFit{})
}

/*
fitBivariateWithPrior warm-starts from a prior joint fit and scans a local neighborhood.
*/
func fitBivariateWithPrior(
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
	prior BivariateFit,
) BivariateFit {
	context, ok := newFitContext(buyEvents, sellEvents, horizon)

	if !ok || !context.enoughEvents(buyEvents, sellEvents) {
		return BivariateFit{}
	}

	muBuyStart := float64(context.BuyEvents) / context.SpanSec
	muSellStart := float64(context.SellEvents) / context.SpanSec

	if muBuyStart <= 0 {
		muBuyStart = 1 / context.SpanSec
	}

	if muSellStart <= 0 {
		muSellStart = 1 / context.SpanSec
	}

	optimized := optimizeBivariate(
		buyEvents, sellEvents, horizon, context, prior,
	)

	grid := scanBivariateGrid(
		buyEvents, sellEvents, horizon, context,
		muBuyStart, muSellStart, false,
	)

	crossGrid := scanBivariateGrid(
		buyEvents, sellEvents, horizon, context,
		muBuyStart, muSellStart, true,
	)

	return bestBivariateFit(
		buyEvents, sellEvents, horizon, optimized, grid, crossGrid,
	)
}

func scanBivariateGrid(
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
	context FitContext,
	muBuyStart, muSellStart float64,
	crossOnly bool,
) BivariateFit {
	best := BivariateFit{}
	bestLL := math.Inf(-1)

	for _, beta := range context.BetaCandidates {
		for _, muBuyFactor := range context.MuBuyFactors {
			muBuy := muBuyStart * muBuyFactor

			for _, muSellFactor := range context.MuSellFactors {
				muSell := muSellStart * muSellFactor

				for _, branchBB := range context.BranchSelfCandidates {
					for _, branchSS := range context.BranchSelfCandidates {
						for _, branchBS := range context.BranchCrossCandidates {
							for _, branchSB := range context.BranchCrossCandidates {
								if crossOnly {
									if branchBS <= 0 && branchSB <= 0 {
										continue
									}
								}

								if branchBB <= 0 && branchSS <= 0 && branchBS <= 0 && branchSB <= 0 {
									continue
								}

								crossCap := crossBranchShare(
									math.Max(branchBB, branchSS),
									context.BranchCeiling,
								)

								if branchBS > crossCap || branchSB > crossCap {
									continue
								}

								spectral := spectralRadius(
									branchBB*beta, branchBS*beta,
									branchSB*beta, branchSS*beta,
									beta,
								)

								if spectral <= context.BranchFloor || spectral >= criticalBranch {
									continue
								}

								candidate := evaluateBivariateCandidate(
									buyEvents, sellEvents, horizon, context,
									muBuy, muSell,
									branchBB*beta, branchBS*beta,
									branchSB*beta, branchSS*beta,
									beta,
								)

								if candidate.fit.MuBuy <= 0 || candidate.logLikelihood <= bestLL {
									continue
								}

								bestLL = candidate.logLikelihood
								best = candidate.fit
							}
						}
					}
				}
			}
		}
	}

	return best
}

func bestBivariateFit(
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
	candidates ...BivariateFit,
) BivariateFit {
	best := BivariateFit{}
	bestLL := math.Inf(-1)

	for _, candidate := range candidates {
		if candidate.MuBuy <= 0 {
			continue
		}

		logLikelihood := bivariateLogLikelihood(
			buyEvents, sellEvents, horizon,
			candidate.MuBuy, candidate.MuSell,
			candidate.AlphaBB, candidate.AlphaBS,
			candidate.AlphaSB, candidate.AlphaSS,
			candidate.Beta,
		)

		if !betterBivariateCandidate(best, candidate, bestLL, logLikelihood) {
			continue
		}

		if !crossLikelihoodValid(buyEvents, sellEvents, horizon, candidate) {
			continue
		}

		bestLL = logLikelihood
		best = candidate
	}

	return best
}

func crossLikelihoodValid(
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
	fit BivariateFit,
) bool {
	if fit.AlphaBS <= 0 && fit.AlphaSB <= 0 {
		return true
	}

	full := bivariateLogLikelihood(
		buyEvents, sellEvents, horizon,
		fit.MuBuy, fit.MuSell,
		fit.AlphaBB, fit.AlphaBS, fit.AlphaSB, fit.AlphaSS,
		fit.Beta,
	)
	restricted := bivariateLogLikelihood(
		buyEvents, sellEvents, horizon,
		fit.MuBuy, fit.MuSell,
		fit.AlphaBB, 0, 0, fit.AlphaSS,
		fit.Beta,
	)

	return full+1e-9 >= restricted
}

func betterBivariateCandidate(
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

func evaluateBivariateCandidate(
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
	context FitContext,
	muBuy, muSell, alphaBB, alphaBS, alphaSB, alphaSS, beta float64,
) fitCandidate {
	fit := evaluateBivariateFit(
		buyEvents, sellEvents, horizon, context,
		muBuy, muSell, alphaBB, alphaBS, alphaSB, alphaSS, beta,
	)

	if fit.MuBuy <= 0 {
		return fitCandidate{}
	}

	logLikelihood := bivariateLogLikelihood(
		buyEvents, sellEvents, horizon,
		muBuy, muSell, alphaBB, alphaBS, alphaSB, alphaSS, beta,
	)

	return fitCandidate{
		fit:           fit,
		logLikelihood: logLikelihood,
	}
}
