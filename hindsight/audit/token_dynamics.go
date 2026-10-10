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
The verdict is SUPPORTED when the held-out transition entropy is lower than
the block-shuffled null at the significance level (add-one empirical p),
i.e. the token sequence carries order beyond its dwell structure.
*/
func AnalyzeTokenDynamics(
	frozenGrid *store.Grid,
	unseenTicks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	permutations int,
	significance float64,
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
	var compressedTokens []string
	compressedTokensBySymbol := make(map[string][]string)
	compressedTokenFreqs := make(map[string]int)
	compressedTransitions := make(map[string]map[string]int)
	stayCount := 0
	totalTransitions := 0

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
			if len(symMeas) == 0 {
				continue
			}

			// Assemble unified frame with peers, exactly mirroring production strategy.Training:
			frame := data.NewMeasurement(
				symMeas[0].Epoch,
				sym,
				"dynamics",
				symMeas[0].SeqIdx,
				tick,
			)
			frame.At = symMeas[0].At
			frame.From = symMeas[0].From
			frame.Peers(symMeas...)
			frame.Write()

			tokenBytes := frozenGrid.Observe(frame)
			token := string(tokenBytes)

			regions := frozenGrid.RegionScores(frame)

			// A frame in which no region holds a metric is not an emission.
			if regions.Winner == 0 {
				continue
			}

			winningRegion := regions.Winner
			maxBrightness := regions.Brightness[winningRegion]
			// Without a contending region the margin is measured against the
			// null expectation, which is brightness 0 for every region.
			runnerUpBrightness := 0.0

			if regions.RunnerUp != 0 {
				runnerUpBrightness = regions.Brightness[regions.RunnerUp]
			}

			tokens = append(tokens, token)
			tokensBySymbol[sym] = append(tokensBySymbol[sym], token)
			tokenFreqs[token]++

			// Track run-length compressed tokens (mirroring S3 sequence storage in training.Train):
			symComp := compressedTokensBySymbol[sym]
			if len(symComp) == 0 || symComp[len(symComp)-1] != token {
				if len(symComp) > 0 {
					prevComp := symComp[len(symComp)-1]
					if compressedTransitions[prevComp] == nil {
						compressedTransitions[prevComp] = make(map[string]int)
					}
					compressedTransitions[prevComp][token]++
				}
				compressedTokensBySymbol[sym] = append(compressedTokensBySymbol[sym], token)
				compressedTokens = append(compressedTokens, token)
				compressedTokenFreqs[token]++
			}

			margin := maxBrightness - runnerUpBrightness
			winningScore := maxBrightness
			active := float64(regions.Counts[winningRegion])
			members := float64(regions.Counts[winningRegion])
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

			// Brightness is negative for a frame quieter than the null, so the
			// first emission seeds the peak rather than a zero floor.
			if len(tokens) == 1 || winningScore > peakScore {
				peakScore = winningScore
			}

			sumCoverage += coverage
			sumMargin += margin

			prevToken := prevTokens[sym]

			if prevToken != "" {
				totalTransitions++
				if prevToken == token {
					stayCount++
				}
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

	alwaysStayAccuracy := 0.0
	if totalTransitions > 0 {
		alwaysStayAccuracy = float64(stayCount) / float64(totalTransitions)
	}

	marginalAccuracy := maxDominance

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

	// Compute compressed sequence transition entropy and null:
	compEntropy := 0.0
	compNullMean := 0.0
	compReduction := 0.0
	if len(compressedTokens) > 0 {
		compEntropy = computeTransitionEntropy(compressedTransitions, compressedTokenFreqs, len(compressedTokens))
		compNullEntropies := computeBlockNullTransitionEntropies(compressedTokensBySymbol, 1, permutations)
		for _, value := range compNullEntropies {
			compNullMean += value
		}
		if len(compNullEntropies) > 0 {
			compNullMean /= float64(len(compNullEntropies))
		}
		compReduction = compNullMean - compEntropy
	}

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

	pValue := lowerPValue(realEntropy, nullEntropies)
	verdict := hypothesisVerdict(pValue, len(nullEntropies), significance)

	return Stage4TokenDynamics{
		TotalEmissions:                 len(tokens),
		UniqueTokens:                   len(tokenFreqs),
		TokenFrequencies:               tokenFreqs,
		MaxTokenDominance:              maxDominance,
		MeanExcitationStrength:         meanStrength,
		PeakExcitationStrength:         peakScore,
		MeanActiveCoverage:             meanActiveCov,
		MeanRunnerUpMargin:             meanMargin,
		RegionStrengths:                regionStrengths,
		TransitionEntropy:              realEntropy,
		NullTransitionEntropy:          nullMean,
		EntropyReductionBits:           entropyReduction,
		Transitions:                    transitions,
		CompressedEmissions:            len(compressedTokens),
		CompressedUniqueTokens:         len(compressedTokenFreqs),
		CompressedTransitions:          compressedTransitions,
		CompressedTransitionEntropy:    compEntropy,
		CompressedNullEntropy:          compNullMean,
		CompressedEntropyReductionBits: compReduction,
		AlwaysStayAccuracy:             alwaysStayAccuracy,
		MarginalAccuracy:               marginalAccuracy,
		SummaryText: fmt.Sprintf(
			"Token Dynamics (held out): %d raw emissions (%d compressed changes, dwell stay=%.1f%%). "+
				"Raw entropy = %.3f vs null %.3f (gain=%.3f bits; block=%d; null-rank=%.1f%%; stay-baseline=%.1f%%, marginal-baseline=%.1f%%). "+
				"Compressed transition entropy = %.3f vs shuffled null %.3f (gain=%.3f bits).",
			len(tokens), len(compressedTokens), alwaysStayAccuracy*100,
			realEntropy, nullMean, entropyReduction, blockSize, nullRank*100, alwaysStayAccuracy*100, marginalAccuracy*100,
			compEntropy, compNullMean, compReduction,
		),
		PValue: pValue,
		Status: verdict,
		Passed: passed(verdict),
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

	for range iterations {
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
