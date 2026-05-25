package probability

import (
	"math/rand"
	"sort"
)

/*
SortDescending orders ranked probabilities highest-first, breaking ties alphabetically.
*/
func SortDescending(distribution []Ranked) {
	sort.Slice(distribution, func(leftIndex int, rightIndex int) bool {
		left := distribution[leftIndex]
		right := distribution[rightIndex]

		if left.Probability == right.Probability {
			return left.Token < right.Token
		}

		return left.Probability > right.Probability
	})
}

/*
Sample draws one token from a ranked distribution using the provided RNG.
Temperature 0 picks uniformly among tied maxima; positive temperature
does standard CDF sampling.
*/
func Sample(distribution []Ranked, temperature float64, rng *rand.Rand) string {
	if len(distribution) == 0 {
		return ""
	}

	if temperature == 0 {
		return sampleGreedy(distribution, rng)
	}

	value := rng.Float64()
	running := 0.0

	for _, entry := range distribution {
		running += entry.Probability

		if value <= running {
			return entry.Token
		}
	}

	return distribution[len(distribution)-1].Token
}

func sampleGreedy(distribution []Ranked, rng *rand.Rand) string {
	maxProb := distribution[0].Probability
	best := make([]string, 0, len(distribution))

	for _, entry := range distribution {
		if entry.Probability > maxProb {
			maxProb = entry.Probability
			best = best[:0]
		}

		if entry.Probability == maxProb {
			best = append(best, entry.Token)
		}
	}

	return best[rng.Intn(len(best))]
}
