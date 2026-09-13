package hawkes

import (
	"math"

	"gonum.org/v1/gonum/optimize"
)

const (
	lbfgsMemory          = 12
	lbfgsMajorIterations = 80
)

/*
bivariateEstimator fits joint Hawkes parameters from an arrival stream via
multi-start L-BFGS maximum likelihood.
*/
type bivariateEstimator struct {
	prior bivariateFit
}

/*
newBivariateEstimator constructs an estimator with an optional warm-start
prior.
*/
func newBivariateEstimator(prior bivariateFit) *bivariateEstimator {
	return &bivariateEstimator{prior: prior}
}

/*
fit estimates parameters via multi-start L-BFGS on the exact log-likelihood.
*/
func (estimator *bivariateEstimator) fit(
	stream arrivalStream,
	horizonSec float64,
) bivariateFit {
	return estimator.fitRestricted(stream, horizonSec, fitUnrestricted)
}

/*
fitSelfOnly re-estimates the baseline, decay, and diagonal excitation while
constraining both cross-excitation terms to zero. It provides the correct
restricted likelihood reference for testing whether cross excitation adds
explanatory power.
*/
func (estimator *bivariateEstimator) fitSelfOnly(
	stream arrivalStream,
	horizonSec float64,
) bivariateFit {
	return estimator.fitRestricted(stream, horizonSec, fitSelfOnly)
}

func (estimator *bivariateEstimator) fitRestricted(
	stream arrivalStream,
	horizonSec float64,
	restriction fitRestriction,
) bivariateFit {
	context, ok := newFitContext(stream, horizonSec)

	if !ok || !context.enoughEvents(stream) {
		return bivariateFit{}
	}

	best := bivariateFit{}
	bestLL := math.Inf(-1)
	poisson := context.poissonFit().withIntensitiesAt(stream, horizonSec)

	if poisson.valid() {
		best = poisson
		bestLL = poisson.logLikelihood(stream, horizonSec)
	}

	for _, seed := range estimator.multiStartSeeds(context) {
		candidate := estimator.maximizeLikelihoodRestricted(
			stream, horizonSec, context, seed, restriction,
		)

		if candidate.muX <= 0 {
			continue
		}

		if restriction == fitUnrestricted &&
			!estimator.crossLikelihoodValid(stream, horizonSec, candidate) {
			candidate = candidate.withCrossZeroed().withIntensitiesAt(stream, horizonSec)
		}

		logLikelihood := candidate.logLikelihood(stream, horizonSec)

		if !estimator.preferCandidate(best, candidate, bestLL, logLikelihood) {
			continue
		}

		bestLL = logLikelihood
		best = candidate
	}

	return best
}

func (estimator *bivariateEstimator) crossLikelihoodValid(
	stream arrivalStream,
	horizonSec float64,
	fit bivariateFit,
) bool {
	if fit.alphaXY <= 0 && fit.alphaYX <= 0 {
		return true
	}

	restricted := bivariateFit{
		muX:     fit.muX,
		muY:     fit.muY,
		alphaXX: fit.alphaXX,
		alphaYY: fit.alphaYY,
		beta:    fit.beta,
	}

	fitLL := fit.logLikelihood(stream, horizonSec)
	restrictedLL := restricted.logLikelihood(stream, horizonSec)

	return fitLL+logLikelihoodTolerance(fitLL, restrictedLL) >= restrictedLL
}

func (estimator *bivariateEstimator) preferCandidate(
	current, candidate bivariateFit,
	currentLL, candidateLL float64,
) bool {
	if candidate.muX <= 0 {
		return false
	}

	if current.muX <= 0 {
		return true
	}

	improvementTolerance := logLikelihoodTolerance(currentLL, candidateLL)

	return candidateLL > currentLL+improvementTolerance
}

/*
logLikelihoodTolerance scales a machine-epsilon-derived tolerance by the
larger magnitude among the compared log-likelihoods, so candidate comparisons
are decisive relative to floating-point noise at whatever scale the fit's own
likelihood happens to sit at, never a fixed absolute cutoff.
*/
func logLikelihoodTolerance(values ...float64) float64 {
	scale := 1.0

	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}

		absValue := math.Abs(value)

		if absValue > scale {
			scale = absValue
		}
	}

	radicand := math.Nextafter(1, 2) - 1
	radicandScale := math.Max(1, math.Abs(radicand))
	tolerance := 32 * radicand * radicandScale

	if radicand < -tolerance {
		panic("hawkes: machine-epsilon radicand is negative beyond tolerance")
	}

	return math.Sqrt(math.Max(0, radicand)) * scale
}

func (estimator *bivariateEstimator) maximizeLikelihoodRestricted(
	stream arrivalStream,
	horizonSec float64,
	context fitContext,
	start [bivariateParamCount]float64,
	restriction fitRestriction,
) bivariateFit {
	bounds, err := context.logParamBounds()

	if err != nil {
		panic(err)
	}

	freeStart := bounds.encode(start)
	problem := optimize.Problem{
		Func: func(free []float64) float64 {
			value, _, ok := estimator.negLogLikelihoodRestricted(
				free, bounds, stream, horizonSec, context, restriction,
			)

			if !ok {
				return math.Inf(1)
			}

			return value
		},
		Grad: func(grad, free []float64) {
			_, naturalGrad, ok := estimator.negLogLikelihoodGradRestricted(
				free, bounds, stream, horizonSec, context, restriction,
			)

			if !ok {
				for index := range grad {
					grad[index] = 0
				}

				return
			}

			jacobian := bounds.softplusJacobian(free)

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

	if err != nil || (result.Status != optimize.Success && result.Status != optimize.GradientThreshold) {
		return bivariateFit{}
	}

	fit := fitFromRestrictedLogParams(bounds.decode(result.X), context, restriction)

	if fit.muX <= 0 {
		return bivariateFit{}
	}

	return fit.withIntensitiesAt(stream, horizonSec)
}

func (estimator *bivariateEstimator) negLogLikelihoodRestricted(
	free []float64,
	bounds logParamBounds,
	stream arrivalStream,
	horizonSec float64,
	context fitContext,
	restriction fitRestriction,
) (float64, bivariateFit, bool) {
	fit := fitFromRestrictedLogParams(bounds.decode(free), context, restriction)

	if fit.muX <= 0 {
		return math.Inf(1), bivariateFit{}, false
	}

	logLikelihood, _, ok := fit.logLikelihoodGradient(stream, horizonSec)

	if !ok {
		return math.Inf(1), bivariateFit{}, false
	}

	return -logLikelihood, fit, true
}

func (estimator *bivariateEstimator) negLogLikelihoodGradRestricted(
	free []float64,
	bounds logParamBounds,
	stream arrivalStream,
	horizonSec float64,
	context fitContext,
	restriction fitRestriction,
) (float64, [bivariateParamCount]float64, bool) {
	fit := fitFromRestrictedLogParams(bounds.decode(free), context, restriction)

	if fit.muX <= 0 {
		return math.Inf(1), [bivariateParamCount]float64{}, false
	}

	logLikelihood, naturalGradient, ok := fit.logLikelihoodGradient(stream, horizonSec)

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

func (estimator *bivariateEstimator) multiStartSeeds(
	context fitContext,
) [][bivariateParamCount]float64 {
	muXStart := context.muXStart()
	muYStart := context.muYStart()
	betaStart := 1 / context.medianGapSec
	selfBranchSeed := math.Max(context.branchFloor, selfBranchShareFromContext(context)*context.branchCeiling)
	crossBranchSeed, err := crossBranchFloorFromContext(context)

	if err != nil {
		panic(err)
	}

	baseLog := [bivariateParamCount]float64{
		logPositive(muXStart),
		logPositive(muYStart),
		logPositive(betaStart),
		logPositive(selfBranchSeed),
		logPositive(crossBranchSeed),
		logPositive(crossBranchSeed),
		logPositive(selfBranchSeed),
	}
	seeds := make([][bivariateParamCount]float64, 0, len(context.localScales)+2)

	if estimator.prior.valid() {
		if priorSeed, ok := logParamsFromFit(estimator.prior); ok {
			seeds = append(seeds, priorSeed)
		}
	}

	seeds = append(seeds, baseLog)

	for _, scale := range context.localScales {
		perturbed := baseLog
		perturbed[3] += math.Log(scale)
		perturbed[4] += math.Log(scale)
		perturbed[5] += math.Log(scale)
		perturbed[6] += math.Log(scale)
		seeds = append(seeds, perturbed)
	}

	return seeds
}

func logParamsFromFit(fit bivariateFit) ([bivariateParamCount]float64, bool) {
	beta := fit.beta

	if beta <= 0 {
		return [bivariateParamCount]float64{}, false
	}

	return [bivariateParamCount]float64{
		logPositive(fit.muX),
		logPositive(fit.muY),
		logPositive(fit.beta),
		logPositive(fit.alphaXX / beta),
		logPositive(fit.alphaXY / beta),
		logPositive(fit.alphaYX / beta),
		logPositive(fit.alphaYY / beta),
	}, true
}
