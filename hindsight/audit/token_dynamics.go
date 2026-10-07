package audit

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/theapemachine/symm/nomagique/store"
)

/*
AnalyzeTokenDynamics passes observations through a settled grid and evaluates
token dominance, transition matrix structure, and conditional entropy vs shuffled null.
*/
func AnalyzeTokenDynamics(
	grid *store.Grid,
	ticks []int64,
	series map[string]map[int64]float64,
) Stage4TokenDynamics {
	if grid == nil || grid.RegionCount() == 0 {
		return Stage4TokenDynamics{
			SummaryText: "Grid has no formed regions; cannot emit tokens.",
			Passed:      false,
		}
	}

	var tokens []string
	tokenFreqs := make(map[string]int)
	transitions := make(map[string]map[string]int)

	prevToken := ""
	for _, tick := range ticks {
		pass := make(map[string]store.Excitation)

		for name, tickMap := range series {
			if val, ok := tickMap[tick]; ok {
				pass[store.CellKey(name)] = store.Excitation{
					Deformation: val,
					Confidence:  1.0,
				}
			}
		}

		lit := grid.LitRegion(pass)
		if len(lit) == 0 {
			continue
		}

		tokenStr := fmt.Sprintf("R%d", lit[0])
		tokens = append(tokens, tokenStr)
		tokenFreqs[tokenStr]++

		if prevToken != "" {
			if transitions[prevToken] == nil {
				transitions[prevToken] = make(map[string]int)
			}
			transitions[prevToken][tokenStr]++
		}

		prevToken = tokenStr
	}

	totalEmissions := len(tokens)
	if totalEmissions < 20 {
		return Stage4TokenDynamics{
			TotalEmissions: totalEmissions,
			SummaryText:    "Insufficient token emissions to evaluate dynamics.",
			Passed:         false,
		}
	}

	maxDominance := 0.0
	for _, count := range tokenFreqs {
		share := float64(count) / float64(totalEmissions)
		if share > maxDominance {
			maxDominance = share
		}
	}

	realEntropy := computeTransitionEntropy(transitions, tokenFreqs, totalEmissions)

	// Null transition entropy: shuffle the token sequence to measure expected entropy under randomness
	nullEntropy := computeNullTransitionEntropy(tokens, 30)
	entropyReduction := nullEntropy - realEntropy

	// Healthy conditions: no single token dominates >80%, and real entropy is lower than shuffled null
	passed := maxDominance < 0.80 && entropyReduction >= 0.10

	summary := fmt.Sprintf(
		"Token Dynamics: %d emissions across %d regions. Max dominance = %.1f%%. Transition entropy = %.3f bits vs Null = %.3f bits (reduction = %.3f bits).",
		totalEmissions, len(tokenFreqs), maxDominance*100, realEntropy, nullEntropy, entropyReduction,
	)

	return Stage4TokenDynamics{
		TotalEmissions:        totalEmissions,
		UniqueTokens:          len(tokenFreqs),
		TokenFrequencies:      tokenFreqs,
		MaxTokenDominance:     maxDominance,
		TransitionEntropy:     realEntropy,
		NullTransitionEntropy: nullEntropy,
		EntropyReductionBits:  entropyReduction,
		Transitions:           transitions,
		SummaryText:           summary,
		Passed:                passed,
	}
}

func computeTransitionEntropy(
	transitions map[string]map[string]int,
	tokenFreqs map[string]int,
	totalEmissions int,
) float64 {
	condEntropy := 0.0

	for fromToken, nextMap := range transitions {
		fromCount := tokenFreqs[fromToken]
		if fromCount == 0 {
			continue
		}

		probFrom := float64(fromCount) / float64(totalEmissions)
		rowEntropy := 0.0

		rowTotal := 0
		for _, count := range nextMap {
			rowTotal += count
		}

		for _, count := range nextMap {
			if count > 0 && rowTotal > 0 {
				probNext := float64(count) / float64(rowTotal)
				rowEntropy -= probNext * math.Log2(probNext)
			}
		}

		condEntropy += probFrom * rowEntropy
	}

	return condEntropy
}

func computeNullTransitionEntropy(tokens []string, iterations int) float64 {
	if len(tokens) < 2 {
		return 0
	}

	total := len(tokens)
	sumEntropy := 0.0
	rng := rand.New(rand.NewSource(1791))

	shuffled := make([]string, total)

	for iter := 0; iter < iterations; iter++ {
		copy(shuffled, tokens)
		rng.Shuffle(total, func(first, second int) {
			shuffled[first], shuffled[second] = shuffled[second], shuffled[first]
		})

		nullFreqs := make(map[string]int)
		nullTransitions := make(map[string]map[string]int)

		for idx := 0; idx < total-1; idx++ {
			fromToken := shuffled[idx]
			toToken := shuffled[idx+1]

			nullFreqs[fromToken]++
			if nullTransitions[fromToken] == nil {
				nullTransitions[fromToken] = make(map[string]int)
			}
			nullTransitions[fromToken][toToken]++
		}

		nullFreqs[shuffled[total-1]]++
		sumEntropy += computeTransitionEntropy(nullTransitions, nullFreqs, total)
	}

	return sumEntropy / float64(iterations)
}
