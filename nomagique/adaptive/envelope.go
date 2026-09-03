package adaptive

import (
	"math"
)

/*
EnvelopeType defines the adaptive support frontier strategy.
*/
type EnvelopeType int

const (
	EVT EnvelopeType = iota
	CHEBYSHEV_ENVELOPE
	GAUSSIAN_TAIL
)

// MinimumSampleCountForEnvelope is the degrees of freedom threshold required before bounding samples (n >= 2).
const MinimumSampleCountForEnvelope = 2.0

/*
Envelope establishes dynamic support frontiers using Extreme Value Theory (EVT)
or distribution-free statistical bounds rather than arbitrary minimum/maximum clamps.
Implements the closed Node contract: Step(Number) Number.
*/
type Envelope struct {
	Type EnvelopeType

	count   float64
	welford WelfordEngine
	lower   float64
	upper   float64
}

// Step bounds the incoming sample within the emergent adaptive frontier.
func (envelope *Envelope) Step(number Number) Number {
	val := float64(number)
	envelope.count++
	mean, stdDev := envelope.welford.Update(val)

	if envelope.count < MinimumSampleCountForEnvelope || stdDev <= 0 {
		return number
	}

	switch envelope.Type {
	case EVT:
		// Extreme Value Theory: asymptotic maximum frontier for independent samples
		// E[max(X_1...X_n)] = mu + sigma * sqrt(2 * ln(n)) (Embrechts et al., 1997)
		margin := stdDev * math.Sqrt(2.0*math.Log(envelope.count))
		envelope.lower = mean - margin
		envelope.upper = mean + margin

	case CHEBYSHEV_ENVELOPE:
		// Distribution-free Chebyshev boundary for alpha = 1/count: k = sqrt(count)
		k := math.Sqrt(envelope.count)
		envelope.lower = mean - k*stdDev
		envelope.upper = mean + k*stdDev

	case GAUSSIAN_TAIL:
		// Empirical Gaussian tail boundary adapting to sample depth: k = sqrt(2 * ln(count))
		k := math.Sqrt(2.0 * math.Log(envelope.count))
		envelope.lower = mean - k*stdDev
		envelope.upper = mean + k*stdDev

	default:
		k := math.Sqrt(envelope.count)
		envelope.lower = mean - k*stdDev
		envelope.upper = mean + k*stdDev
	}

	// Dynamic clamping within emergent support envelope
	if val < envelope.lower {
		return Number(envelope.lower)
	}

	if val > envelope.upper {
		return Number(envelope.upper)
	}

	return number
}
