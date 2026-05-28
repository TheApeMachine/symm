package hawkes

import (
	"math"
	"time"

	"github.com/theapemachine/symm/numeric/decay"
	"gonum.org/v1/gonum/optimize"
)

const (
	lbfgsMemory          = 12
	lbfgsMajorIterations = 80
)

type logParamBounds struct {
	lower [bivariateParamCount]float64
	upper [bivariateParamCount]float64
}

func (context FitContext) logParamBounds() logParamBounds {
	betaMin := context.BetaCandidates[0]
	betaMax := context.BetaCandidates[len(context.BetaCandidates)-1]
	selfMax := context.BranchCeiling * selfBranchShareFromContext(context)
	crossMax := context.BranchCeiling
	minRate := 1 / context.SpanSec
	maxRate := float64(context.TotalEvents) / context.SpanSec

	return logParamBounds{
		lower: [bivariateParamCount]float64{
			decay.LogPositive(minRate),
			decay.LogPositive(minRate),
			math.Log(betaMin),
			decay.LogPositive(context.BranchFloor),
			decay.LogPositive(1e-9),
			decay.LogPositive(1e-9),
			decay.LogPositive(context.BranchFloor),
		},
		upper: [bivariateParamCount]float64{
			decay.LogPositive(maxRate),
			decay.LogPositive(maxRate),
			math.Log(betaMax),
			math.Log(selfMax),
			math.Log(crossMax),
			math.Log(crossMax),
			math.Log(selfMax),
		},
	}
}

func selfBranchShareFromContext(context FitContext) float64 {
	tune := arrivalTune{
		totalEvents: context.TotalEvents,
		buyEvents:   context.BuyEvents,
		sellEvents:  context.SellEvents,
	}

	return tune.selfBranchShare()
}

func (bounds logParamBounds) decode(free []float64) [bivariateParamCount]float64 {
	params := [bivariateParamCount]float64{}

	for index := range free {
		sigmoid := 1 / (1 + math.Exp(-free[index]))
		params[index] = bounds.lower[index] + (bounds.upper[index]-bounds.lower[index])*sigmoid
	}

	return params
}

func (bounds logParamBounds) encode(params [bivariateParamCount]float64) []float64 {
	free := make([]float64, bivariateParamCount)

	for index := range params {
		span := bounds.upper[index] - bounds.lower[index]

		if span <= 0 {
			free[index] = 0
			continue
		}

		ratio := (params[index] - bounds.lower[index]) / span
		ratio = math.Max(1e-9, math.Min(1-1e-9, ratio))
		free[index] = math.Log(ratio / (1 - ratio))
	}

	return free
}

func (bounds logParamBounds) sigmoidJacobian(free []float64) [bivariateParamCount]float64 {
	jacobian := [bivariateParamCount]float64{}

	for index := range free {
		sigmoid := 1 / (1 + math.Exp(-free[index]))
		jacobian[index] = (bounds.upper[index] - bounds.lower[index]) * sigmoid * (1 - sigmoid)
	}

	return jacobian
}

func fitFromLogParams(
	logParams [bivariateParamCount]float64,
	context FitContext,
) BivariateFit {
	muBuy := math.Exp(logParams[0])
	muSell := math.Exp(logParams[1])
	beta := math.Exp(logParams[2])
	branchBB := math.Exp(logParams[3])
	branchBS := math.Exp(logParams[4])
	branchSB := math.Exp(logParams[5])
	branchSS := math.Exp(logParams[6])

	if branchBB > context.BranchCeiling || branchSS > context.BranchCeiling {
		return BivariateFit{}
	}

	crossCap := context.CrossBranchCap(math.Max(branchBB, branchSS))

	if branchBS > crossCap || branchSB > crossCap {
		return BivariateFit{}
	}

	fit := BivariateFit{
		MuBuy:   muBuy,
		MuSell:  muSell,
		AlphaBB: branchBB * beta,
		AlphaBS: branchBS * beta,
		AlphaSB: branchSB * beta,
		AlphaSS: branchSS * beta,
		Beta:    beta,
	}
	fit.SpectralRadius = fit.computeSpectralRadius()

	if fit.SpectralRadius <= context.BranchFloor || fit.SpectralRadius >= criticalBranch {
		return BivariateFit{}
	}

	return fit
}

func (estimator *BivariateEstimator) maximizeLikelihood(
	stream ArrivalStream,
	horizon time.Time,
	context FitContext,
	start [bivariateParamCount]float64,
) BivariateFit {
	bounds := context.logParamBounds()
	freeStart := bounds.encode(start)
	problem := optimize.Problem{
		Func: func(free []float64) float64 {
			value, _, ok := estimator.negLogLikelihood(free, bounds, stream, horizon, context)

			if !ok {
				return math.Inf(1)
			}

			return value
		},
		Grad: func(grad, free []float64) {
			_, naturalGrad, ok := estimator.negLogLikelihoodGrad(
				free, bounds, stream, horizon, context,
			)

			if !ok {
				for index := range grad {
					grad[index] = 0
				}

				return
			}

			jacobian := bounds.sigmoidJacobian(free)

			for index := range grad {
				grad[index] = naturalGrad[index] * jacobian[index]
			}
		},
	}
	result, err := optimize.Minimize(
		problem,
		freeStart,
		&optimize.Settings{
			GradientThreshold: 1e-6,
			MajorIterations:   lbfgsMajorIterations,
		},
		&optimize.LBFGS{Store: lbfgsMemory},
	)

	if err != nil {
		return BivariateFit{}
	}

	fit := fitFromLogParams(bounds.decode(result.X), context)

	if fit.MuBuy <= 0 {
		return BivariateFit{}
	}

	return fit.WithIntensitiesAt(stream, horizon)
}

func (estimator *BivariateEstimator) negLogLikelihood(
	free []float64,
	bounds logParamBounds,
	stream ArrivalStream,
	horizon time.Time,
	context FitContext,
) (float64, BivariateFit, bool) {
	fit := fitFromLogParams(bounds.decode(free), context)

	if fit.MuBuy <= 0 {
		return math.Inf(1), BivariateFit{}, false
	}

	logLikelihood, _, ok := fit.LogLikelihoodGradient(stream, horizon)

	if !ok {
		return math.Inf(1), BivariateFit{}, false
	}

	return -logLikelihood, fit, true
}

func (estimator *BivariateEstimator) negLogLikelihoodGrad(
	free []float64,
	bounds logParamBounds,
	stream ArrivalStream,
	horizon time.Time,
	context FitContext,
) (float64, [bivariateParamCount]float64, bool) {
	fit := fitFromLogParams(bounds.decode(free), context)

	if fit.MuBuy <= 0 {
		return math.Inf(1), [bivariateParamCount]float64{}, false
	}

	logLikelihood, naturalGradient, ok := fit.LogLikelihoodGradient(stream, horizon)

	if !ok {
		return math.Inf(1), [bivariateParamCount]float64{}, false
	}

	logGrad := logSpaceGradient(naturalGradient, fit)
	negGrad := [bivariateParamCount]float64{}

	for index := range logGrad {
		negGrad[index] = -logGrad[index]
	}

	return -logLikelihood, negGrad, true
}

func (estimator *BivariateEstimator) multiStartSeeds(
	context FitContext,
) [][bivariateParamCount]float64 {
	muBuyStart := context.MuBuyStart()
	muSellStart := context.MuSellStart()
	betaStart := 1 / context.MedianGapSec
	baseLog := [bivariateParamCount]float64{
		decay.LogPositive(muBuyStart),
		decay.LogPositive(muSellStart),
		decay.LogPositive(betaStart),
		decay.LogPositive(0.2),
		decay.LogPositive(0.05),
		decay.LogPositive(0.05),
		decay.LogPositive(0.2),
	}
	seeds := make([][bivariateParamCount]float64, 0, len(context.LocalScales)+2)

	if estimator.prior.valid() {
		seeds = append(seeds, logParamsFromFit(estimator.prior))
	}

	seeds = append(seeds, baseLog)

	for _, scale := range context.LocalScales {
		perturbed := baseLog
		perturbed[3] += math.Log(scale)
		perturbed[4] += math.Log(scale)
		perturbed[5] += math.Log(scale)
		perturbed[6] += math.Log(scale)
		seeds = append(seeds, perturbed)
	}

	return seeds
}

func logParamsFromFit(fit BivariateFit) [bivariateParamCount]float64 {
	beta := fit.Beta

	if beta <= 0 {
		beta = 1
	}

	return [bivariateParamCount]float64{
		decay.LogPositive(fit.MuBuy),
		decay.LogPositive(fit.MuSell),
		decay.LogPositive(fit.Beta),
		decay.LogPositive(fit.AlphaBB / beta),
		decay.LogPositive(fit.AlphaBS / beta),
		decay.LogPositive(fit.AlphaSB / beta),
		decay.LogPositive(fit.AlphaSS / beta),
	}
}
