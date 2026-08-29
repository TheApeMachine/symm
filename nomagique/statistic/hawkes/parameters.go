package hawkes

import "math"

/*
fitRestriction selects the nested statistical model optimized against the
same observation window and data-derived parameter bounds.
*/
type fitRestriction int

const (
	fitUnrestricted fitRestriction = iota
	fitSelfOnly
)

/*
fitFromLogParams is the unrestricted decoder used by the production full-model
path.
*/
func fitFromLogParams(
	logParams [bivariateParamCount]float64,
	context fitContext,
) bivariateFit {
	return fitFromRestrictedLogParams(logParams, context, fitUnrestricted)
}

/*
fitFromRestrictedLogParams converts bounded log coordinates
[muX, muY, beta, branchXX, branchXY, branchYX, branchYY] into Hawkes
parameters and applies the nested-model restriction before stability
testing. This keeps the self-only likelihood comparison on the same fitted
domain as the full model rather than zeroing coefficients after
optimization.
*/
func fitFromRestrictedLogParams(
	logParams [bivariateParamCount]float64,
	context fitContext,
	restriction fitRestriction,
) bivariateFit {
	muX := math.Exp(logParams[0])
	muY := math.Exp(logParams[1])
	beta := math.Exp(logParams[2])
	branchXX := math.Exp(logParams[3])
	branchXY := math.Exp(logParams[4])
	branchYX := math.Exp(logParams[5])
	branchYY := math.Exp(logParams[6])

	if branchXX > context.branchCeiling || branchYY > context.branchCeiling {
		return bivariateFit{}
	}

	crossCap := context.crossBranchCap(math.Max(branchXX, branchYY))

	if restriction == fitUnrestricted && (branchXY > crossCap || branchYX > crossCap) {
		return bivariateFit{}
	}

	fit := bivariateFit{
		muX:     muX,
		muY:     muY,
		alphaXX: branchXX * beta,
		alphaXY: branchXY * beta,
		alphaYX: branchYX * beta,
		alphaYY: branchYY * beta,
		beta:    beta,
	}

	if restriction == fitSelfOnly {
		fit.alphaXY = 0
		fit.alphaYX = 0
	}

	fit.spectralRadius = fit.computeSpectralRadius()

	if fit.spectralRadius < 0 || fit.spectralRadius >= criticalBranch {
		return bivariateFit{}
	}

	return fit
}
