package audit

import (
	"fmt"
	"math/rand"
	"slices"
)

/*
insufficient is the violation of a stage that could not evaluate: too little
evidence is a failed audit, never a pass.
*/
func insufficient(stage, description string) *Violation {
	return NewViolation(stage, stage+".insufficient_evidence", description, nil, nil)
}

/*
permutationNull compares observed against statistic over runs label shuffles
of tokens split at mid (seeded, so a rerun reproduces). It returns the
permutation p-value (1 + exceed) / (runs + 1), and the null mean and 95th
percentile.
*/
func permutationNull(
	tokens []string, mid int, observed float64, runs int,
	statistic func(first, second []string) float64,
) (pValue, mean, p95 float64) {
	shuffled := slices.Clone(tokens)
	null := make([]float64, runs)
	exceed := 0
	rng := rand.New(rand.NewSource(17))

	for run := range runs {
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		null[run] = statistic(shuffled[:mid], shuffled[mid:])
		mean += null[run]

		if null[run] >= observed {
			exceed++
		}
	}

	slices.Sort(null)
	p95Idx := min(int(0.95*float64(runs)), runs-1)

	return float64(1+exceed) / float64(runs+1), mean / float64(runs), null[p95Idx]
}

func tokenJSD(first, second []string) float64 {
	return computeJSD(computeFrequencyDistribution(first), computeFrequencyDistribution(second))
}

func regionToken(key uint8) string {
	return fmt.Sprintf("R%02d", key)
}
