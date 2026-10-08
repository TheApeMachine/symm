package hawkes

import "math"

/*
criticalBranch is the spectral-radius boundary a Hawkes process must stay
strictly below to be stable: at or above it, expected offspring per event
diverges and the process is no longer stationary.
*/
const criticalBranch = 1.0

/*
branchingMatrix returns the 2x2 branching matrix G = A/beta for one bivariate
parameter set: column j gives the expected first-generation offspring on each
side triggered by one event on side j.
*/
func branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta float64) [2][2]float64 {
	invBeta := 1 / beta

	return [2][2]float64{
		{alphaXX * invBeta, alphaXY * invBeta},
		{alphaYX * invBeta, alphaYY * invBeta},
	}
}

/*
spectralRadius returns the spectral radius of a 2x2 branching matrix.
Complex eigenvalues use modulus; real eigenvalues use maximum absolute value.
*/
func spectralRadius(matrix [2][2]float64) float64 {
	trace := matrix[0][0] + matrix[1][1]
	determinant := matrix[0][0]*matrix[1][1] - matrix[0][1]*matrix[1][0]
	discriminant := trace*trace - 4*determinant

	if discriminant < 0 {
		modulus := math.Sqrt(-discriminant)
		realPart := trace / 2
		imagPart := modulus / 2

		return math.Sqrt(realPart*realPart + imagPart*imagPart)
	}

	rootHigh := (trace + math.Sqrt(discriminant)) / 2
	rootLow := (trace - math.Sqrt(discriminant)) / 2

	return math.Max(math.Abs(rootHigh), math.Abs(rootLow))
}

/*
meanIntensity returns the stationary mean intensities implied by a bivariate
branching matrix and baseline rates, solving (I-G)*lambda = mu.
*/
func meanIntensity(
	muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, beta float64,
) (lambdaX float64, lambdaY float64, ok bool) {
	if beta <= 0 {
		return 0, 0, false
	}

	branching := branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta)
	determinant := (1-branching[0][0])*(1-branching[1][1]) - branching[0][1]*branching[1][0]

	if determinant <= 0 {
		return 0, 0, false
	}

	lambdaX = ((1-branching[1][1])*muX + branching[0][1]*muY) / determinant
	lambdaY = (branching[1][0]*muX + (1-branching[0][0])*muY) / determinant

	if lambdaX < 0 || lambdaY < 0 || math.IsNaN(lambdaX) || math.IsNaN(lambdaY) {
		return 0, 0, false
	}

	return lambdaX, lambdaY, true
}

/*
finiteNonNegative reports whether a value is finite and non-negative, the
validity requirement for an expected-offspring count.
*/
func finiteNonNegative(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

/*
immediateOffspring returns the expected first-generation children caused by
one buy parent and one sell parent, using column sums of the branching matrix
(columns identify the parent stream).
*/
func immediateOffspring(
	alphaXX, alphaXY, alphaYX, alphaYY, beta float64,
) (buyParent float64, sellParent float64, ok bool) {
	buyParent = (alphaXX + alphaYX) / beta
	sellParent = (alphaXY + alphaYY) / beta

	if !finiteNonNegative(buyParent) || !finiteNonNegative(sellParent) {
		return 0, 0, false
	}

	return buyParent, sellParent, true
}

/*
totalDescendants returns all expected descendants across every generation for
one parent on each side: for a stable process this is the column sum of
(I-G)^-1 minus the original parent.
*/
func totalDescendants(
	alphaXX, alphaXY, alphaYX, alphaYY, beta float64,
) (buyParent float64, sellParent float64, ok bool) {
	branch := branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta)
	determinant := (1-branch[0][0])*(1-branch[1][1]) - branch[0][1]*branch[1][0]

	if determinant <= 0 {
		return 0, 0, false
	}

	buyParent = (1-branch[1][1]+branch[1][0])/determinant - 1
	sellParent = (branch[0][1]+1-branch[0][0])/determinant - 1

	if !finiteNonNegative(buyParent) || !finiteNonNegative(sellParent) {
		return 0, 0, false
	}

	return buyParent, sellParent, true
}
