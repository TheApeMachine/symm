package probability

import (
	"math"

	"github.com/theapemachine/errnie"
)

func SoftmaxScoresNormalized(scores []float64) ([]float64, error) {
	if len(scores) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"softmax: requires at least one score",
			nil,
		))
	}

	maxMagnitude := 0.0

	for index, score := range scores {
		if math.IsNaN(score) || math.IsInf(score, 0) {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"softmax: score is non-finite",
				nil,
			).With("index", index))
		}

		maxMagnitude = math.Max(maxMagnitude, math.Abs(score))
	}

	if len(scores) == 1 {
		return []float64{1}, nil
	}

	_, scaleExponent := math.Frexp(maxMagnitude)
	scaledScores := make([]float64, len(scores))
	mean := 0.0

	for index, score := range scores {
		scaledScore := math.Ldexp(score, -scaleExponent)
		scaledScores[index] = scaledScore
		mean += scaledScore
	}

	mean /= float64(len(scores))

	variance := 0.0

	for _, score := range scaledScores {
		deviation := score - mean
		variance += deviation * deviation
	}

	variance /= float64(len(scores) - 1)
	stddev := math.Sqrt(variance)

	if stddev <= 0 {
		share := 1.0 / float64(len(scores))
		probabilities := make([]float64, len(scores))

		for index := range probabilities {
			probabilities[index] = share
		}

		return probabilities, nil
	}

	for index, score := range scaledScores {
		scaledScores[index] = (score - mean) / stddev
	}

	maxScore := scaledScores[0]

	for _, score := range scaledScores[1:] {
		maxScore = math.Max(maxScore, score)
	}

	probabilities := make([]float64, len(scores))
	expSum := 0.0

	for index, score := range scaledScores {
		weighted := math.Exp(score - maxScore)
		probabilities[index] = weighted
		expSum += weighted
	}

	if expSum <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"softmax: normalization sum is non-positive",
			nil,
		))
	}

	for index := range probabilities {
		probabilities[index] /= expSum
	}

	return probabilities, nil
}
