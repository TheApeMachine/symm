package audit

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/strategy"
)

/*
AnalyzeTokenDynamics evaluates token emissions and transition structure on unseen held-out
data using a frozen grid. Compares transition entropy against a block-shuffled temporal null
that preserves ordinary local autocorrelation/stickiness.
*/
func AnalyzeTokenDynamics(
	frozenGrid *store.Grid,
	unseenTicks []int64,
	tickMeasurements map[int64][]*data.Measurement,
) Stage4TokenDynamics {
	if frozenGrid == nil || frozenGrid.RegionCount() == 0 {
		return Stage4TokenDynamics{
			SummaryText: "Grid has no formed regions; cannot emit tokens.",
			Passed:      false,
		}
	}

	stream := store.NewStream()
	var tokens []string
	tokenFreqs := make(map[string]int)
	transitions := make(map[string]map[string]int)

	prevToken := ""
	for _, tick := range unseenTicks {
		measGroup := tickMeasurements[tick]
		if len(measGroup) == 0 {
			continue
		}

		observed := strategy.ChannelsFrom(measGroup...)
		deforms := stream.Deform(observed.Raw)
		excited := observed.Excite(deforms)

		lit := frozenGrid.LitRegion(excited)
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
			SummaryText:    "Insufficient token emissions on out-of-sample data to evaluate dynamics.",
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

	// Block-shuffled temporal null: preserves local persistence (block size 4) while randomizing transition sequence
	nullEntropy := computeBlockNullTransitionEntropy(tokens, 4, 30)
	entropyReduction := nullEntropy - realEntropy

	passed := maxDominance < 0.80 && entropyReduction >= 0.05

	summary := fmt.Sprintf(
		"Token Dynamics (Out-of-Sample): %d emissions across %d regions. Max dominance = %.1f%%. "+
			"Transition entropy = %.3f bits vs Block-Null = %.3f bits (reduction = %.3f bits).",
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

func computeBlockNullTransitionEntropy(tokens []string, blockSize int, iterations int) float64 {
	total := len(tokens)
	if total < blockSize*2 {
		return computeSimpleNullTransitionEntropy(tokens, iterations)
	}

	// Split tokens into blocks
	var blocks [][]string
	for i := 0; i < total; i += blockSize {
		end := min(i+blockSize, total)
		blocks = append(blocks, tokens[i:end])
	}

	rng := rand.New(rand.NewSource(1791))
	sumEntropy := 0.0

	for iter := 0; iter < iterations; iter++ {
		shuffledBlocks := make([][]string, len(blocks))
		copy(shuffledBlocks, blocks)
		rng.Shuffle(len(shuffledBlocks), func(first, second int) {
			shuffledBlocks[first], shuffledBlocks[second] = shuffledBlocks[second], shuffledBlocks[first]
		})

		var shuffled []string
		for _, blk := range shuffledBlocks {
			shuffled = append(shuffled, blk...)
		}

		nullFreqs := make(map[string]int)
		nullTransitions := make(map[string]map[string]int)

		for idx := 0; idx < len(shuffled)-1; idx++ {
			fromToken := shuffled[idx]
			toToken := shuffled[idx+1]

			nullFreqs[fromToken]++
			if nullTransitions[fromToken] == nil {
				nullTransitions[fromToken] = make(map[string]int)
			}
			nullTransitions[fromToken][toToken]++
		}

		nullFreqs[shuffled[len(shuffled)-1]]++
		sumEntropy += computeTransitionEntropy(nullTransitions, nullFreqs, len(shuffled))
	}

	return sumEntropy / float64(iterations)
}

func computeSimpleNullTransitionEntropy(tokens []string, iterations int) float64 {
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
