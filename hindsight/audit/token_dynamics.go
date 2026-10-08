package audit

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/strategy"
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
	streams map[string]*store.Stream,
	unseenTicks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	permutations int,
) Stage4TokenDynamics {
	if frozenGrid == nil || frozenGrid.RegionsFormed() == 0 {
		return Stage4TokenDynamics{
			SummaryText: "Grid/streams unavailable for held-out token dynamics.",
			Status:      "INSUFFICIENT_DATA",
			Passed:      false,
		}
	}

	if streams == nil {
		streams = make(map[string]*store.Stream)
	}

	if permutations <= 0 {
		permutations = 50
	}

	var tokens []string
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
			st, ok := streams[sym]
			if !ok {
				st = store.NewStream()
				streams[sym] = st
			}

			observed := strategy.ChannelsFrom(symMeas...)
			deforms := st.Deform(observed.Raw)
			excited := observed.Excite(deforms)

			scores := frozenGrid.RegionScores(excited)
			if len(scores) == 0 || scores[0].Score <= 0 {
				continue
			}

			winning := scores[0]
			token := fmt.Sprintf("R%d", winning.Region)
			tokens = append(tokens, token)
			tokenFreqs[token]++

			margin := winning.Score
			if len(scores) > 1 {
				margin = winning.Score - scores[1].Score
			}

			accum := regionAccums[token]
			if accum == nil {
				accum = &regionAccumulator{
					minScore: winning.Score,
					maxScore: winning.Score,
				}
				regionAccums[token] = accum
			}

			accum.emissions++
			accum.totalScore += winning.Score

			if winning.Score < accum.minScore {
				accum.minScore = winning.Score
			}

			if winning.Score > accum.maxScore {
				accum.maxScore = winning.Score
			}

			accum.totalActive += float64(winning.Contributors)
			accum.totalMembers += float64(winning.Members)
			accum.totalMargin += margin

			sumScore += winning.Score

			if winning.Score > peakScore {
				peakScore = winning.Score
			}

			sumCoverage += winning.Coverage
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
	nullEntropies := computeBlockNullTransitionEntropies(tokens, blockSize, permutations)

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

func computeBlockNullTransitionEntropies(tokens []string, blockSize, iterations int) []float64 {
	if len(tokens) < 2 || iterations <= 0 {
		return nil
	}
	if blockSize < 1 {
		blockSize = 1
	}

	blocks := make([][]string, 0, (len(tokens)+blockSize-1)/blockSize)
	for start := 0; start < len(tokens); start += blockSize {
		end := min(start+blockSize, len(tokens))
		blocks = append(blocks, append([]string(nil), tokens[start:end]...))
	}

	rng := rand.New(rand.NewSource(1791))
	entropies := make([]float64, 0, iterations)

	for iteration := 0; iteration < iterations; iteration++ {
		shuffledBlocks := append([][]string(nil), blocks...)
		rng.Shuffle(len(shuffledBlocks), func(first, second int) {
			shuffledBlocks[first], shuffledBlocks[second] =
				shuffledBlocks[second], shuffledBlocks[first]
		})

		shuffled := make([]string, 0, len(tokens))
		for _, block := range shuffledBlocks {
			shuffled = append(shuffled, block...)
		}

		frequencies := make(map[string]int)
		transitions := make(map[string]map[string]int)
		for index, token := range shuffled {
			frequencies[token]++
			if index == 0 {
				continue
			}
			previous := shuffled[index-1]
			if transitions[previous] == nil {
				transitions[previous] = make(map[string]int)
			}
			transitions[previous][token]++
		}

		entropies = append(
			entropies,
			computeTransitionEntropy(transitions, frequencies, len(shuffled)),
		)
	}

	return entropies
}
