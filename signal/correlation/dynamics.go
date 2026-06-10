package correlation

import (
	"math"

	"github.com/theapemachine/symm/numeric"
)

func peerLowMagnitudeCorrelation(
	correlation float64,
	lowerCorrelation float64,
	correlationSpread float64,
	peerCorrelations []float64,
) bool {
	if correlationSpread > 0 {
		return math.Abs(correlation) <= lowerCorrelation
	}

	if len(peerCorrelations) < 3 {
		return false
	}

	peerMagnitude := numeric.MedianAbsolute(peerCorrelations)

	if peerMagnitude <= 0 {
		return false
	}

	return math.Abs(correlation) < peerMagnitude
}
