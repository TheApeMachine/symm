package audit

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
AnalyzeTokenDynamics evaluates token emissions and transition structure on unseen
held-out data using a frozen grid.

The Stream is supplied by the caller and is the same causal stream used while
developing the grid, so the holdout boundary does not invent a market-memory
reset that live operation would never experience.

The temporal null block length is derived from the observed token dwell lengths.
No entropy-reduction threshold is converted into a health verdict.
*/
func AnalyzeTokenDynamics(
	frozenGrid *store.Grid,
	unseenTicks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	permutations int,
) Stage4TokenDynamics {
	if frozenGrid == nil {
		return Stage4TokenDynamics{
			SummaryText: "Grid unavailable for held-out token dynamics.",
			Status:      "INSUFFICIENT_DATA",
			Passed:      false,
		}
	}

	if permutations <= 0 {
		permutations = 50
	}

	var tokens []string
	tokensBySymbol := make(map[string][]string)
	tokenFreqs := make(map[string]int)
	transitions := make(map[string]map[string]int)

	type regionAccumulator struct {
		emissions    int
		totalScore   float64
		minScore     float64
		maxScore     float64
		totalActive  float64
		totalMembers float64
		totalMargin  float64
	}

	regionAccums := make(map[string]*regionAccumulator)
	sumScore := 0.0
	peakScore := 0.0
	sumCoverage := 0.0
	sumMargin := 0.0

	prevTokens := make(map[string]string)

	for _, tick := range unseenTicks {
		measGroup := tickMeasurements[tick]

		if len(measGroup) == 0 {
			continue
		}

		bySymbol := make(map[string][]*data.Measurement)

		for _, m := range measGroup {
			if m != nil {
				bySymbol[m.Label] = append(bySymbol[m.Label], m)
			}
		}

		for sym, symMeas := range bySymbol {
			var regScores [13]float64
			var regCounts [13]int

			for _, m := range symMeas {
				source := m.Source

				for entry := range m.Read() {
					if entry == nil || entry.Metric == nil {
						continue
					}

					region := frozenGrid.PinRegion(source, entry.Key)
					regScores[region] += math.Abs(entry.Metric.Standardized)
					regCounts[region]++
				}
			}

			winningRegion := uint8(2)
			maxBrightness := 0.0
			runnerUpBrightness := 0.0

			for r := uint8(1); r <= 12; r++ {
				if regCounts[r] == 0 {
					continue
				}

				brightness := regScores[r] / math.Sqrt(float64(regCounts[r]))

				if brightness > maxBrightness {
					runnerUpBrightness = maxBrightness
					maxBrightness = brightness
					winningRegion = r
					continue
				}

				if brightness > runnerUpBrightness {
					runnerUpBrightness = brightness
				}
			}

			if maxBrightness <= 0 {
				continue
			}

			token := fmt.Sprintf("R%02d", winningRegion)
			tokens = append(tokens, token)
			tokensBySymbol[sym] = append(tokensBySymbol[sym], token)
			tokenFreqs[token]++

			margin := maxBrightness - runnerUpBrightness
			winningScore := maxBrightness
			active := float64(regCounts[winningRegion])
			members := float64(regCounts[winningRegion])
			coverage := 1.0

			accum := regionAccums[token]

			if accum == nil {
				accum = &regionAccumulator{
					minScore: winningScore,
					maxScore: winningScore,
				}
				regionAccums[token] = accum
			}

			accum.emissions++
			accum.totalScore += winningScore

			if winningScore < accum.minScore {
				accum.minScore = winningScore
			}

			if winningScore > accum.maxScore {
				accum.maxScore = winningScore
			}

			accum.totalActive += active
			accum.totalMembers += members
			accum.totalMargin += margin

			sumScore += winningScore

			if winningScore > peakScore {
				peakScore = winningScore
			}

			sumCoverage += coverage
			sumMargin += margin

			prevToken := prevTokens[sym]

			if prevToken != "" {
				if transitions[prevToken] == nil {
					transitions[prevToken] = make(map[string]int)
				}
				transitions[prevToken][token]++
			}
			prevTokens[sym] = token
		}
	}

	if len(tokens) < 20 {
		return Stage4TokenDynamics{
			TotalEmissions: len(tokens),
			SummaryText:    "Insufficient held-out token emissions to evaluate dynamics.",
			Status:         "INSUFFICIENT_DATA",
			Passed:         false,
		}
	}

	maxDominance := 0.0
	for _, count := range tokenFreqs {
		share := float64(count) / float64(len(tokens))
		if share > maxDominance {
			maxDominance = share
		}
	}

	regionStrengths := make(map[string]RegionStrengthStat, len(regionAccums))
	for token, accum := range regionAccums {
		meanScore := accum.totalScore / float64(accum.emissions)
		meanActive := accum.totalActive / float64(accum.emissions)
		meanMembers := accum.totalMembers / float64(accum.emissions)
		meanCoverage := 0.0

		if meanMembers > 0 {
			meanCoverage = meanActive / meanMembers
		}

		meanMargin := accum.totalMargin / float64(accum.emissions)

		regionStrengths[token] = RegionStrengthStat{
			Region:       token,
			Emissions:    accum.emissions,
			MeanScore:    meanScore,
			MinScore:     accum.minScore,
			MaxScore:     accum.maxScore,
			MeanActive:   meanActive,
			MeanMembers:  meanMembers,
			MeanCoverage: meanCoverage,
			MeanMargin:   meanMargin,
		}
	}

	meanStrength := 0.0
	meanActiveCov := 0.0
	meanMargin := 0.0

	if len(tokens) > 0 {
		meanStrength = sumScore / float64(len(tokens))
		meanActiveCov = sumCoverage / float64(len(tokens))
		meanMargin = sumMargin / float64(len(tokens))
	}

	realEntropy := computeTransitionEntropy(transitions, tokenFreqs, len(tokens))
	blockSize := empiricalDwellBlockSize(tokens)
	nullEntropies := computeBlockNullTransitionEntropies(tokensBySymbol, blockSize, permutations)

	nullMean := 0.0
	for _, value := range nullEntropies {
		nullMean += value
	}
	if len(nullEntropies) > 0 {
		nullMean /= float64(len(nullEntropies))
	}
	entropyReduction := nullMean - realEntropy

	nullRank := 0.0
	if len(nullEntropies) > 0 {
		greaterOrEqual := 0
		for _, value := range nullEntropies {
			if value >= realEntropy {
				greaterOrEqual++
			}
		}
		nullRank = float64(greaterOrEqual) / float64(len(nullEntropies))
	}

	return Stage4TokenDynamics{
		TotalEmissions:         len(tokens),
		UniqueTokens:           len(tokenFreqs),
		TokenFrequencies:       tokenFreqs,
		MaxTokenDominance:      maxDominance,
		MeanExcitationStrength: meanStrength,
		PeakExcitationStrength: peakScore,
		MeanActiveCoverage:     meanActiveCov,
		MeanRunnerUpMargin:     meanMargin,
		RegionStrengths:        regionStrengths,
		TransitionEntropy:      realEntropy,
		NullTransitionEntropy:  nullMean,
		EntropyReductionBits:   entropyReduction,
		Transitions:            transitions,
		SummaryText: fmt.Sprintf(
			"Token Dynamics (held out): %d emissions across %d regions. "+
				"Mean excitation strength = %.3f (peak = %.3f; runner-up margin = %.3f). "+
				"Max dominance = %.1f%%. Transition entropy = %.3f bits vs empirical dwell-block null mean %.3f "+
				"(difference %.3f bits; block=%d; real entropy <= %.1f%% of null draws).",
			len(tokens), len(tokenFreqs), meanStrength, peakScore, meanMargin,
			maxDominance*100, realEntropy, nullMean,
			entropyReduction, blockSize, nullRank*100,
		),
		Status: "MEASURED",
		Passed: true,
	}
}

func computeTransitionEntropy(
	transitions map[string]map[string]int,
	tokenFreqs map[string]int,
	totalEmissions int,
) float64 {
	conditionalEntropy := 0.0

	for fromToken, nextMap := range transitions {
		fromCount := tokenFreqs[fromToken]
		if fromCount == 0 {
			continue
		}

		rowTotal := 0
		for _, count := range nextMap {
			rowTotal += count
		}
		if rowTotal == 0 {
			continue
		}

		rowEntropy := 0.0
		for _, count := range nextMap {
			if count == 0 {
				continue
			}
			probability := float64(count) / float64(rowTotal)
			rowEntropy -= probability * math.Log2(probability)
		}

		conditionalEntropy +=
			(float64(fromCount) / float64(totalEmissions)) * rowEntropy
	}

	return conditionalEntropy
}

/*
empiricalDwellBlockSize uses the median run length of identical consecutive
tokens as the null's local-persistence scale. The value therefore comes from the
observed sequence rather than a hand-picked constant.
*/
func empiricalDwellBlockSize(tokens []string) int {
	if len(tokens) == 0 {
		return 1
	}

	runs := make([]int, 0)
	runLength := 1
	for index := 1; index < len(tokens); index++ {
		if tokens[index] == tokens[index-1] {
			runLength++
			continue
		}
		runs = append(runs, runLength)
		runLength = 1
	}
	runs = append(runs, runLength)
	sort.Ints(runs)

	median := runs[len(runs)/2]
	if median < 1 {
		return 1
	}
	return median
}

func computeBlockNullTransitionEntropies(
	tokensBySymbol map[string][]string,
	blockSize int,
	iterations int,
) []float64 {
	if iterations <= 0 || len(tokensBySymbol) == 0 {
		return nil
	}

	totalTokens := 0

	for _, symTokens := range tokensBySymbol {
		totalTokens += len(symTokens)
	}

	if totalTokens < 2 {
		return nil
	}

	if blockSize < 1 {
		blockSize = 1
	}

	blocksBySymbol := make(map[string][][]string, len(tokensBySymbol))

	for sym, symTokens := range tokensBySymbol {
		if len(symTokens) == 0 {
			continue
		}

		symBlocks := make([][]string, 0, (len(symTokens)+blockSize-1)/blockSize)

		for start := 0; start < len(symTokens); start += blockSize {
			end := min(start+blockSize, len(symTokens))
			symBlocks = append(symBlocks, append([]string(nil), symTokens[start:end]...))
		}

		blocksBySymbol[sym] = symBlocks
	}

	rng := rand.New(rand.NewSource(1791))
	entropies := make([]float64, 0, iterations)

	for iteration := 0; iteration < iterations; iteration++ {
		frequencies := make(map[string]int)
		transitions := make(map[string]map[string]int)
		totalNullTokens := 0

		for _, symBlocks := range blocksBySymbol {
			if len(symBlocks) == 0 {
				continue
			}

			shuffledBlocks := append([][]string(nil), symBlocks...)
			rng.Shuffle(len(shuffledBlocks), func(first, second int) {
				shuffledBlocks[first], shuffledBlocks[second] =
					shuffledBlocks[second], shuffledBlocks[first]
			})

			shuffledSym := make([]string, 0)

			for _, blk := range shuffledBlocks {
				shuffledSym = append(shuffledSym, blk...)
			}

			for idx, tok := range shuffledSym {
				frequencies[tok]++
				totalNullTokens++

				if idx == 0 {
					continue
				}

				previous := shuffledSym[idx-1]

				if transitions[previous] == nil {
					transitions[previous] = make(map[string]int)
				}

				transitions[previous][tok]++
			}
		}

		entropies = append(
			entropies,
			computeTransitionEntropy(transitions, frequencies, totalNullTokens),
		)
	}

	return entropies
}
