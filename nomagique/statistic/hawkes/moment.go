package hawkes

import (
	"errors"
	"math"

	"gonum.org/v1/gonum/stat"
)

/*
momentDiagnostic validates bivariate exponential-kernel parameters through
empirical moments.
*/
type momentDiagnostic struct {
	momentR float64
	momentS float64
}

/*
momentSample carries aligned count streams and the resulting empirical vs.
estimated moment evidence.
*/
type momentSample struct {
	x       []float64
	y       []float64
	weights []float64
}

/*
momentResult carries empirical and estimated moment evidence.
*/
type momentResult struct {
	value      float64
	empirical  float64
	estimate   float64
	confidence float64
}

/*
newMomentDiagnostic creates a typed Hawkes moment diagnostic.
*/
func newMomentDiagnostic(momentR, momentS float64) (momentDiagnostic, error) {
	if momentR < 0 || momentS < 0 || momentR+momentS == 0 {
		return momentDiagnostic{}, errors.New(
			"hawkes-moment: momentR and momentS must define a nonzero moment",
		)
	}

	return momentDiagnostic{momentR: momentR, momentS: momentS}, nil
}

/*
measure evaluates empirical moments against a fitted process's implied
branching moments.
*/
func (diagnostic momentDiagnostic) measure(
	sample momentSample,
	muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, beta float64,
) (momentResult, error) {
	if len(sample.x) != len(sample.y) || len(sample.x) < 2 {
		return momentResult{}, errors.New(
			"hawkes-moment: require aligned sample streams of at least two observations",
		)
	}

	if len(sample.weights) != 0 && len(sample.weights) != len(sample.x) {
		return momentResult{}, errors.New(
			"hawkes-moment: weights must align with sample streams",
		)
	}

	empirical := stat.BivariateMoment(diagnostic.momentR, diagnostic.momentS, sample.x, sample.y, sample.weights)
	estimate, estimateOK := branchingMomentEstimate(
		muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, beta,
		diagnostic.momentR, diagnostic.momentS,
	)

	if !estimateOK {
		return momentResult{}, errors.New(
			"hawkes-moment: branching moment estimate unavailable for parameters",
		)
	}

	confidence, confidenceOK := momentConfidence(empirical, estimate)

	if !confidenceOK {
		return momentResult{}, errors.New("hawkes-moment: confidence could not be derived")
	}

	return momentResult{
		value:      confidence,
		empirical:  empirical,
		estimate:   estimate,
		confidence: confidence,
	}, nil
}

/*
methodOfMoments derives a stable seed from empirical x and y count streams,
by matching the process's stationary variance/covariance to the branching
matrix implied by one candidate decay rate.
*/
func methodOfMoments(
	x, y, weights []float64, beta float64,
) (muX, muY, alphaXX, alphaXY, alphaYX, alphaYY float64, ok bool) {
	if beta <= 0 || len(x) != len(y) || len(x) < 2 {
		return 0, 0, 0, 0, 0, 0, false
	}

	meanX := stat.Mean(x, weights)
	meanY := stat.Mean(y, weights)

	if meanX <= 0 || meanY <= 0 {
		return 0, 0, 0, 0, 0, 0, false
	}

	secondMomentX := stat.BivariateMoment(2, 0, x, y, weights)
	secondMomentY := stat.BivariateMoment(0, 2, x, y, weights)
	centralVarianceX := secondMomentX - meanX*meanX
	centralVarianceY := secondMomentY - meanY*meanY
	covariance := stat.BivariateMoment(1, 1, x, y, weights)

	if centralVarianceX > meanX {
		alphaXX = (centralVarianceX - meanX) * beta / (2 * meanX)
	}

	if centralVarianceY > meanY {
		alphaYY = (centralVarianceY - meanY) * beta / (2 * meanY)
	}

	if covariance > 0 {
		alphaXY = covariance * beta / (2 * meanY)
		alphaYX = covariance * beta / (2 * meanX)
	}

	branchXX := alphaXX / beta
	branchXY := alphaXY / beta
	branchYX := alphaYX / beta
	branchYY := alphaYY / beta

	muX = meanX - branchXX*meanX - branchXY*meanY
	muY = meanY - branchYX*meanX - branchYY*meanY

	if muX <= 0 || muY <= 0 {
		return 0, 0, 0, 0, 0, 0, false
	}

	if spectralRadius(branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta)) >= criticalBranch {
		return 0, 0, 0, 0, 0, 0, false
	}

	return muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, true
}

/*
branchingMomentEstimate returns the moment-scale diagnostic a fitted process
implies for one mixed moment (2,0), (0,2), or (1,1). It is not an exact
Hawkes central moment; exact count moments require the observation window.
*/
func branchingMomentEstimate(
	muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, beta float64,
	momentR, momentS float64,
) (float64, bool) {
	lambdaX, lambdaY, ok := meanIntensity(muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, beta)

	if !ok {
		return 0, false
	}

	branching := branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	switch {
	case momentR == 2 && momentS == 0:
		return lambdaX + 2*branching[0][0]*lambdaX, true
	case momentR == 0 && momentS == 2:
		return lambdaY + 2*branching[1][1]*lambdaY, true
	case momentR == 1 && momentS == 1:
		return branching[0][1]*lambdaY + branching[1][0]*lambdaX, true
	default:
		return 0, false
	}
}

/*
momentConfidence returns a fit score in (0, 1] from empirical and estimated
moments: 1 when they agree exactly, approaching 0 as their relative residual
grows.
*/
func momentConfidence(empirical, estimate float64) (float64, bool) {
	scale := math.Max(math.Abs(estimate), math.Abs(empirical))

	if scale <= 0 {
		return 0, false
	}

	residual := math.Abs(empirical-estimate) / scale

	return 1 / (1 + residual), true
}

/*
crossAsymmetry compares third-order mixed moments between x and y streams.
*/
func crossAsymmetry(x, y, weights []float64) (float64, bool) {
	if len(x) != len(y) || len(x) < 2 {
		return 0, false
	}

	moment21 := stat.BivariateMoment(2, 1, x, y, weights)
	moment12 := stat.BivariateMoment(1, 2, x, y, weights)

	return moment21 - moment12, true
}
