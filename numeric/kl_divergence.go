package numeric

import "math"

/*
KLDivergence returns sum(q_i * log(q_i / p_i)) for aligned distributions.
Lengths may differ: missing indices use floor on q and p. rowSum <= 0 on the
expected side is lifted to floor before normalization.
*/
func KLDivergence(
	observed []float64,
	expected []float64,
	expectedSum float64,
	floor float64,
) float64 {
	if floor <= 0 {
		floor = 1e-6
	}

	if expectedSum <= 0 {
		expectedSum = expected[0]

		if expectedSum <= 0 {
			expectedSum = floor
		}
	}

	observedSum := 0.0
	width := max(len(observed), len(expected))

	for index := range len(observed) {
		observedSum += observed[index]
	}

	if observedSum <= 0 {
		observedSum = floor
	}

	divergence := 0.0

	for index := range width {
		observedProbability := floor

		if index < len(observed) {
			observedProbability = observed[index] / observedSum
		}

		if observedProbability < floor {
			observedProbability = floor
		}

		expectedMass := floor

		if index < len(expected) {
			expectedMass = expected[index]
		}

		expectedProbability := expectedMass / expectedSum

		if expectedProbability < floor {
			expectedProbability = floor
		}

		divergence += observedProbability * math.Log(observedProbability/expectedProbability)
	}

	return divergence
}
