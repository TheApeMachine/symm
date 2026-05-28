package adaptive

import "math"

type Shannon struct{}

func NewShannon() *Shannon {
	return &Shannon{}
}

/*
ShannonEntropy calculates the Shannon entropy of a vector of 64-bit values.
*/
func (shannon *Shannon) Entropy(vector [8]uint64) float64 {
	var sum float64

	for _, value := range vector {
		sum += float64(value)
	}

	if sum == 0 {
		return 0
	}

	var entropy float64

	for _, value := range vector {
		probability := float64(value) / sum

		if probability > 0 {
			entropy -= probability * math.Log2(probability)
		}
	}

	return entropy
}

/*
ShannonEntropyBitsFromMap sums -p log2(p) for map values interpreted as
nonnegative quantities scaled into probabilities by probabilityScale.
Classifier scores in this repo are percentages; callers pass 1/100 for scale.
*/
func (shannon *Shannon) EntropyBitsFromMap(scores map[string]float64, probabilityScale float64) float64 {
	if probabilityScale <= 0 {
		return math.NaN()
	}

	for _, raw := range scores {
		if raw < 0 {
			return math.NaN()
		}
	}

	var entropy float64

	for _, raw := range scores {
		probability := raw * probabilityScale

		if probability > 0 {
			entropy -= probability * math.Log2(probability)
		}
	}

	return entropy
}
