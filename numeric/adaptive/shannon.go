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

/*
ShannonConsensus returns 1 − H / log2(N) where H is Shannon entropy in
bits over the label histogram and N is the number of distinct observed
labels. The result is 1 when every observed slot carries the same
label (total consensus) and 0 when the labels are uniformly spread.
An empty or single-bucket histogram returns 1 so a newly-seeded field
does not get artificially penalised for lack of diversity.
*/
func (shannon *Shannon) Consensus(histogram map[uint16]int) float64 {
	if len(histogram) <= 1 {
		return 1
	}

	var total float64

	for _, count := range histogram {
		total += float64(count)
	}

	if total == 0 {
		return 1
	}

	var entropy float64

	for _, count := range histogram {
		p := float64(count) / total

		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}

	maxEntropy := math.Log2(float64(len(histogram)))

	if maxEntropy == 0 {
		return 1
	}

	return 1 - entropy/maxEntropy
}
