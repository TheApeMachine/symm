package manifold

import (
	"math"
	"math/cmplx"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
ScanPhaseSpectrum rotates the resident HCAM fingerprint through the full phase
circle and scores it against every stored attractor at each angle. The result
is the Superposition Prism: destructive interference rules out false
continuations and constructive interference confirms the aligned attractor.
*/
func ScanPhaseSpectrum(
	fingerprint []complex128,
	storedAttractors map[string][]complex128,
	steps int,
) []types.PhaseResponse {
	if steps <= 0 {
		steps = 1
	}

	responses := make([]types.PhaseResponse, steps)
	deltaAlpha := 2.0 * math.Pi / float64(steps)
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)

	for index := 0; index < steps; index++ {
		alpha := float64(index) * deltaAlpha
		rotation := cmplx.Rect(1.0, -alpha)

		bestSimilarity := -1.0
		bestOutcome := types.PhaseOutcome{}

		for label, attractor := range storedAttractors {
			similarity := complexOverlapWithRotation(fingerprint, attractor, rotation)

			if similarity > bestSimilarity {
				bestSimilarity = similarity
				bestOutcome = types.PhaseOutcome{
					Direction: label,
					Return:    similarity,
				}
			}
		}

		responses[index] = types.PhaseResponse{
			Angle:      alpha * 180.0 / math.Pi,
			Similarity: bestSimilarity,
			ObservedAt: observedAt,
			Outcome:    bestOutcome,
		}
	}

	return responses
}

/*
complexOverlapWithRotation computes the normalized complex inner product of the
rotated fingerprint against one attractor. The normalized real part is the
signed similarity, so antipodal attractors score negatively rather than being
reported as a weak positive match.
*/
func complexOverlapWithRotation(
	fingerprint []complex128,
	attractor []complex128,
	rotation complex128,
) float64 {
	length := len(fingerprint)

	if len(attractor) < length {
		length = len(attractor)
	}

	if length == 0 {
		return 0
	}

	var (
		overlap         complex128
		fingerprintNorm float64
		attractorNorm   float64
	)

	for index := 0; index < length; index++ {
		overlap += cmplx.Conj(fingerprint[index]) * attractor[index] * rotation
		fingerprintNorm += real(cmplx.Conj(fingerprint[index]) * fingerprint[index])
		attractorNorm += real(cmplx.Conj(attractor[index]) * attractor[index])
	}

	if fingerprintNorm == 0 || attractorNorm == 0 {
		return 0
	}

	return real(overlap) / math.Sqrt(fingerprintNorm*attractorNorm)
}
