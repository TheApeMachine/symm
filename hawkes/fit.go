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

	if prior.valid() {
		optimized := optimizeBivariate(
			buyEvents, sellEvents, horizon, context, prior,
		)

		if optimized.MuBuy > 0 {
			return optimized
		}
	}

	optimized := optimizeBivariate(
		buyEvents, sellEvents, horizon, context, BivariateFit{},
	)

	if optimized.MuBuy > 0 {
		return optimized
	}

	return scanBivariateFullGrid(
		buyEvents, sellEvents, horizon, context,
		muBuyStart, muSellStart,
	)
}

func scanBivariateLocalGrid(
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
	prior BivariateFit,
	context FitContext,
	muBuyStart, muSellStart float64,
) BivariateFit {
	best := BivariateFit{}
	bestLL := math.Inf(-1)

	for _, betaScale := range context.LocalScales {
		beta := prior.Beta * betaScale

		if beta <= 0 {
			continue
		}

		for _, muBuyScale := range context.LocalScales {
			muBuy := prior.MuBuy * muBuyScale

			if muBuy <= 0 {
				continue
			}

			for _, muSellScale := range context.LocalScales {
				muSell := prior.MuSell * muSellScale

				if muSell <= 0 {
					continue
				}

				for _, alphaScale := range context.LocalScales {
					alphaBB := prior.AlphaBB * alphaScale
					alphaBS := prior.AlphaBS * alphaScale
					alphaSB := prior.AlphaSB * alphaScale
					alphaSS := prior.AlphaSS * alphaScale
					candidate := evaluateBivariateCandidate(
						buyEvents, sellEvents, horizon, context,
						muBuy, muSell, alphaBB, alphaBS, alphaSB, alphaSS, beta,
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

	if best.MuBuy > 0 {
		return best
	}

	return scanBivariateFullGrid(
		buyEvents, sellEvents, horizon, context,
		muBuyStart, muSellStart,
	)
}

func scanBivariateFullGrid(
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
	context FitContext,
	muBuyStart, muSellStart float64,
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
								crossCap := crossBranchShare(
									math.Max(branchBB, branchSS),
									context.BranchCeiling,
								)

								if branchBS > crossCap || branchSB > crossCap {
									continue
								}

								if branchBB <= 0 && branchSS <= 0 && branchBS <= 0 && branchSB <= 0 {
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
