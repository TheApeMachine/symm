package reasoning

import (
	"math"
	"sort"

	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

// maxSeedCategories caps how many distinct signals seed their own strategy branch,
// so a noisy tape with dozens of categories does not explode the initial beam.
const maxSeedCategories = 6

const priceChangeEpsilon = 1e-9

type categorySeedStats struct {
	category            types.CategoryType
	count               int
	forwardReturn       float64
	forwardObservations int
}

type symbolForwardState struct {
	lastPrice float64
	pending   []pendingCategory
}

type pendingCategory struct {
	category types.CategoryType
	price    float64
}

/*
DeriveVocabulary reads the categories that actually occur on the tape and pairs
them with bounded numeric grids. Category seed order is data-derived: realized
forward price movement after a category beats raw frequency, so rare entry signals
are not capped out by common neutral/exit readings before replay scoring starts.
*/
func DeriveVocabulary(rows []types.Measurement) Vocabulary {
	categories := deriveSeedCategories(rows)
	fractions := derivePositionFractions(rows)

	return Vocabulary{
		Categories: categories,
		Regimes: []types.Regime{
			types.RegimeTrending, types.RegimeBullish, types.RegimeChoppy,
		},
		Thresholds: []float64{1.0, 1.5, 2.0},
		Lookbacks:  []int{3, 5, 8},
		PriceMoves: []float64{0.5, 1.0, 2.0},
		Offsets:    []float64{0.01, 0.02, 0.05},
		Fractions:  fractions,
		Durations:  []float64{5, 15, 30},
		Entries:    []reasoning.ActionType{reasoning.ActionMarket, reasoning.ActionLimit},
		Protective: []reasoning.ActionType{
			reasoning.ActionTrailingStop, reasoning.ActionStopLoss, reasoning.ActionTakeProfit,
		},
	}
}

func deriveSeedCategories(rows []types.Measurement) []types.CategoryType {
	stats := categoryForwardStats(rows)
	candidates := make([]categorySeedStats, 0, len(stats))

	for category, candidate := range stats {
		candidate.category = category
		candidates = append(candidates, candidate)
	}

	sort.Slice(candidates, func(firstIndex, secondIndex int) bool {
		first := candidates[firstIndex]
		second := candidates[secondIndex]
		firstScore := first.forwardScore()
		secondScore := second.forwardScore()

		if firstScore != secondScore {
			return firstScore > secondScore
		}

		if first.count != second.count {
			return first.count > second.count
		}

		return first.category < second.category
	})

	if len(candidates) > maxSeedCategories {
		candidates = candidates[:maxSeedCategories]
	}

	categories := make([]types.CategoryType, 0, len(candidates))

	for _, candidate := range candidates {
		categories = append(categories, candidate.category)
	}

	return categories
}

func (stats categorySeedStats) forwardScore() float64 {
	if stats.forwardObservations <= 0 {
		return 0
	}

	return stats.forwardReturn / float64(stats.forwardObservations)
}

func categoryForwardStats(rows []types.Measurement) map[types.CategoryType]categorySeedStats {
	stats := make(map[types.CategoryType]categorySeedStats)
	symbols := make(map[string]*symbolForwardState)

	for _, row := range rows {
		if row.Category != types.CategoryTypeNone {
			candidate := stats[row.Category]
			candidate.count++
			stats[row.Category] = candidate
		}

		if row.Symbol == "" || row.Last <= 0 {
			continue
		}

		state := symbols[row.Symbol]

		if state == nil {
			state = &symbolForwardState{lastPrice: row.Last}
			symbols[row.Symbol] = state
		}

		if math.Abs(state.lastPrice-row.Last) > priceChangeEpsilon {
			scorePendingCategories(stats, state.pending, row.Last)
			state.pending = state.pending[:0]
			state.lastPrice = row.Last
		}

		if row.Category == types.CategoryTypeNone {
			continue
		}

		state.pending = append(state.pending, pendingCategory{
			category: row.Category,
			price:    row.Last,
		})
	}

	for _, state := range symbols {
		if len(state.pending) == 0 {
			continue
		}

		scorePendingCategories(stats, state.pending, state.lastPrice)
		state.pending = state.pending[:0]
	}

	return stats
}

func scorePendingCategories(
	stats map[types.CategoryType]categorySeedStats,
	pending []pendingCategory,
	nextPrice float64,
) {
	if nextPrice <= 0 {
		return
	}

	for _, entry := range pending {
		if entry.price <= 0 {
			continue
		}

		returnValue := math.Log(nextPrice / entry.price)
		candidate := stats[entry.category]
		candidate.forwardReturn += returnValue
		candidate.forwardObservations++
		stats[entry.category] = candidate
	}
}

/*
derivePositionFractions builds capital-deployment multipliers from the tape's SNR
distribution so the search grid tracks how strong signals actually read on data.
*/
func derivePositionFractions(rows []types.Measurement) []float64 {
	peakSNR := 0.0

	for _, row := range rows {
		if row.SNR > peakSNR {
			peakSNR = row.SNR
		}
	}

	fractions := []float64{0.25, 0.5, 0.75, 1.0}

	if peakSNR > 0 {
		step := peakSNR / 4

		if step < 0.25 {
			step = 0.25
		}

		if step > 1.0 {
			step = 1.0
		}

		fractions = append(fractions, step)
	}

	sort.Float64s(fractions)
	unique := fractions[:0]

	for _, fraction := range fractions {
		if fraction <= 0 || fraction > 1 {
			continue
		}

		if len(unique) > 0 && unique[len(unique)-1] == fraction {
			continue
		}

		unique = append(unique, fraction)
	}

	return unique
}
