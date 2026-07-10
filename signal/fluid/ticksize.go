package fluid

import (
	"fmt"
	"math"
	"sort"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
)

/*
resolveBookTickSize chooses the exchange price increment when known, otherwise
the minimum intra-side step, otherwise the configured fallback.
*/
func resolveBookTickSize(
	bidPrices []float64,
	askPrices []float64,
	instrumentTick float64,
	fallback float64,
) (float64, error) {
	if instrumentTick > 0 {
		return instrumentTick, nil
	}

	minStep := 0.0

	for _, sideStep := range []float64{
		minStepFromPrices(bidPrices),
		minStepFromPrices(askPrices),
	} {
		if sideStep <= 0 {
			continue
		}

		if minStep == 0 || sideStep < minStep {
			minStep = sideStep
		}
	}

	if minStep > 0 {
		return minStep, nil
	}

	if fallback > 0 {
		return fallback, nil
	}

	if len(bidPrices) == 0 && len(askPrices) == 0 {
		return 0, fmt.Errorf("fluid: book tick size requires prices or fallback")
	}

	return 0, fmt.Errorf("fluid: book tick size could not be resolved")
}

func minStepFromPrices(prices []float64) float64 {
	if len(prices) < 2 {
		return 0
	}

	sorted := append([]float64(nil), prices...)
	sort.Float64s(sorted)

	minStep := 0.0

	for index := 1; index < len(sorted); index++ {
		step := sorted[index] - sorted[index-1]

		if step <= 0 {
			continue
		}

		if minStep == 0 || step < minStep {
			minStep = step
		}
	}

	return minStep
}

/*
touchBandCells counts lattice cells within the near-touch band around mid.
*/
func touchBandCells(spread, tickSize float64, halfWidth int) int {
	if tickSize <= 0 || spread <= 0 || halfWidth <= 0 {
		return 0
	}

	band := int(math.Ceil(spread / (2 * tickSize)))

	if band < 1 {
		return 1
	}

	if band > halfWidth {
		return halfWidth
	}

	return band
}

func safeIntCeil(value float64) (int, bool) {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}

	if value >= float64(math.MaxInt-1) {
		return 0, false
	}

	return int(math.Ceil(value)), true
}

func capGridHalfWidth(
	derived int,
	configuredMax int,
	bidLevels int,
	askLevels int,
) int {
	capacity := configuredMax

	levelCap := bidLevels

	if askLevels > levelCap {
		levelCap = askLevels
	}

	if levelCap > 0 && (capacity <= 0 || levelCap < capacity) {
		capacity = levelCap
	}

	if derived <= 0 {
		return capacity
	}

	if capacity <= 0 || derived < capacity {
		return derived
	}

	return capacity
}

func gridHalfWidthFromBook(
	bids, asks []kraken.BookLevel,
	tickSize float64,
) int {
	if len(bids) == 0 || len(asks) == 0 || tickSize <= 0 {
		return 0
	}

	mid := (bids[0].Price.Float64() + asks[0].Price.Float64()) / 2
	maxDistance := 0.0

	for _, level := range bids {
		maxDistance = math.Max(maxDistance, math.Abs(level.Price.Float64()-mid))
	}

	for _, level := range asks {
		maxDistance = math.Max(maxDistance, math.Abs(level.Price.Float64()-mid))
	}

	halfWidth, ok := safeIntCeil(maxDistance / tickSize)

	if !ok {
		return 0
	}

	return halfWidth
}

func maxGridCellCount(bookDepth int) int {
	if bookDepth <= 0 {
		return 0
	}

	if bookDepth > (math.MaxInt-1)/2 {
		return 0
	}

	return bookDepth*2 + 1
}

func configuredBookDepthLevels() int {
	bookDepth := viper.GetInt("market.book_depth_levels")

	if bookDepth <= 0 {
		return 0
	}

	return bookDepth
}
