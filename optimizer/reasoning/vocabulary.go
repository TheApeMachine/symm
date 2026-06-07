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

// minSeedObservations is the statistical floor for a category to be rankable at
// all. Without it the mean-forward-return ranking is a lottery for rare
// categories (variance shrinks as 1/√n): on a real 3.4M-row tape a
// 36-observation category outranked 467k-observation ones, and the seed slots
// filled with noise winners whose SNR gates then never fired.
const minSeedObservations = 64

const priceChangeEpsilon = 1e-9

type categorySeedStats struct {
	category            types.CategoryType
	count               int
	forwardReturn       float64
	forwardSquared      float64
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
		Thresholds: deriveThresholds(rows),
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

/*
deriveThresholds grids SNR gates on the tape's ACTUAL SNR distribution (median /
p90 / p99 of category-bearing rows) instead of a static {1, 1.5, 2}. The
surprisal SNR pins a tracker's dominant category to a ~0.5 fixed point and only
transitions spike above it, so a hardcoded ≥1 gate is structurally unfireable
for exactly the categories that dominate a tape (observed live: anchor_stall —
260k rows, zero of them ≥1). Seeding from the median lets seeds actually trade;
mutations can still tighten toward the tails.
*/
func deriveThresholds(rows []types.Measurement) []float64 {
	values := make([]float64, 0, len(rows)/4)

	for index := range rows {
		if rows[index].Category == types.CategoryTypeNone || rows[index].SNR <= 0 {
			continue
		}

		values = append(values, rows[index].SNR)
	}

	if len(values) < minSeedObservations {
		return []float64{1.0, 1.5, 2.0}
	}

	sort.Float64s(values)

	quantile := func(fraction float64) float64 {
		position := int(float64(len(values)-1) * fraction)

		return values[position]
	}

	thresholds := []float64{quantile(0.50), quantile(0.90), quantile(0.99)}
	unique := thresholds[:0]

	for _, threshold := range thresholds {
		rounded := math.Round(threshold*100) / 100

		if rounded <= 0 {
			continue
		}

		if len(unique) > 0 && unique[len(unique)-1] >= rounded {
			continue
		}

		unique = append(unique, rounded)
	}

	if len(unique) == 0 {
		return []float64{1.0, 1.5, 2.0}
	}

	return unique
}

func deriveSeedCategories(rows []types.Measurement) []types.CategoryType {
	stats := categoryForwardStats(rows)
	candidates := make([]categorySeedStats, 0, len(stats))
	floor := seedObservationFloor(len(rows))

	for category, candidate := range stats {
		if candidate.forwardObservations < floor {
			continue
		}

		candidate.category = category
		candidates = append(candidates, candidate)
	}

	// Too few categories clear the floor (tiny fixture tapes): fall back to all,
	// still ranked, so small tests and thin captures keep working.
	if len(candidates) < 2 {
		candidates = candidates[:0]

		for category, candidate := range stats {
			candidate.category = category
			candidates = append(candidates, candidate)
		}
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

// seedObservationFloor scales the rankability floor with tape size, bounded
// below by minSeedObservations.
func seedObservationFloor(rowCount int) int {
	floor := rowCount / 50_000

	if floor < minSeedObservations {
		return minSeedObservations
	}

	return floor
}

/*
forwardScore ranks a category by the t-statistic of its mean forward return —
mean / (std/√n) — not the raw mean. The raw mean rewarded whichever rare
category got lucky: with no variance term a handful of fortunate observations
beat half a million unremarkable ones, and the seed slots filled with
unfireable noise.
*/
func (stats categorySeedStats) forwardScore() float64 {
	n := float64(stats.forwardObservations)

	if n <= 1 {
		return 0
	}

	mean := stats.forwardReturn / n
	variance := stats.forwardSquared/n - mean*mean

	if variance < 1e-18 {
		variance = 1e-18
	}

	return mean / math.Sqrt(variance/n)
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
		candidate.forwardSquared += returnValue * returnValue
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
