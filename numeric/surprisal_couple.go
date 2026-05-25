package numeric

import "math"

const surprisalVelocityMagnitudeEpsilon = 0.01

/*
SurprisalVelocityCouple scores alignment of two scalar surprisal velocities: product
over squared geometric mean magnitude, with a floor so quiescent nodes do not spuriously couple.
*/
func SurprisalVelocityCouple(leftVelocity float64, rightVelocity float64) float64 {
	geometricMean := math.Sqrt(math.Abs(leftVelocity) * math.Abs(rightVelocity))

	if geometricMean < surprisalVelocityMagnitudeEpsilon {
		return 0
	}

	return (leftVelocity * rightVelocity) / (geometricMean * geometricMean)
}
