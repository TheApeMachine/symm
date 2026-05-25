package hawkes

import (
	"math"
	"time"

	"github.com/theapemachine/symm/kraken/trade"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/stats"
)

const (
	bivariateParamCount = 7
	criticalBranch      = 1.0
)

/*
FitContext holds market-derived bounds for one bivariate Hawkes fit.
*/
type FitContext struct {
	SpanSec               float64
	MedianGapSec          float64
	GapLowerSec           float64
	GapUpperSec           float64
	GapCV                 float64
	TotalEvents           int
	BuyEvents             int
	SellEvents            int
	MinFitEvents          int
	MinPerSide            int
	TradeWindow           time.Duration
	ScanSteps             int
	BranchScanSteps       int
	BranchFloor           float64
	BranchCeiling         float64
	BetaCandidates        []float64
	MuBuyFactors          []float64
	MuSellFactors         []float64
	BranchSelfCandidates  []float64
	BranchCrossCandidates []float64
	LocalScales           []float64
}

/*
newFitContext derives fit bounds from observed trade arrivals.
*/
func newFitContext(
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
) (FitContext, bool) {
	marked := mergeMarkedEvents(buyEvents, sellEvents)

	if len(marked) < 2 {
		return FitContext{}, false
	}

	span := windowSpan(marked, horizon)

	if span <= 0 {
		return FitContext{}, false
	}

	gaps := gapsFromMarked(marked)

	if len(gaps) == 0 {
		return FitContext{}, false
	}

	medianGap := stats.Median(gaps)
	lowerGap, upperGap := stats.Quartiles(gaps)

	if medianGap <= 0 {
		return FitContext{}, false
	}

	if upperGap <= lowerGap {
		upperGap = medianGap * (1 + 1/math.Sqrt(float64(len(gaps))))
		lowerGap = medianGap * (1 - 1/math.Sqrt(float64(len(gaps))))

		if lowerGap <= 0 {
			lowerGap = medianGap / 2
		}
	}

	gapSpread := upperGap - lowerGap
	gapCV := gapSpread / medianGap
	total := len(buyEvents) + len(sellEvents)
	minFitEvents := minFitEventsFor(total)
	minPerSide := minEventsPerSide(total)
	scanSteps := scanStepsFromEvents(total)
	branchSteps := branchScanSteps(total)
	branchFloor := branchFloorFor(total)
	branchCeiling := branchCeilingFor(total)
	betaMin := 1 / upperGap
	betaMax := 1 / lowerGap
	localMin, localMax := localScaleRange(gapCV)

	return FitContext{
		SpanSec:      span,
		MedianGapSec: medianGap,
		GapLowerSec:  lowerGap,
		GapUpperSec:  upperGap,
		GapCV:        gapCV,
		TotalEvents:  total,
		BuyEvents:    len(buyEvents),
		SellEvents:   len(sellEvents),
		MinFitEvents: minFitEvents,
		MinPerSide:   minPerSide,
		TradeWindow: tradeWindowDuration(
			medianGap, total, minFitEvents,
		),
		ScanSteps:       scanSteps,
		BranchScanSteps: branchSteps,
		BranchFloor:     branchFloor,
		BranchCeiling:   branchCeiling,
		BetaCandidates: numeric.LogSpace(
			betaMin, betaMax, scanSteps,
		),
		MuBuyFactors: muUncertaintyFactors(
			len(buyEvents), scanSteps,
		),
		MuSellFactors: muUncertaintyFactors(
			len(sellEvents), scanSteps,
		),
		BranchSelfCandidates: numeric.LinSpace(
			branchFloor,
			branchCeiling*selfBranchShare(
				total, len(buyEvents), len(sellEvents),
			),
			branchSteps,
		),
		BranchCrossCandidates: numeric.LinSpace(
			0, branchCeiling, branchSteps,
		),
		LocalScales: numeric.LinSpace(
			localMin, localMax, scanSteps,
		),
	}, true
}

func fitContextFromTicks(
	ticks []trade.Data,
	windowStart, horizon time.Time,
) (FitContext, []time.Time, []time.Time, bool) {
	buyTimes, sellTimes := splitSideEvents(ticks, windowStart, horizon)

	if len(buyTimes)+len(sellTimes) < 2 {
		return FitContext{}, nil, nil, false
	}

	probe, ok := newFitContext(buyTimes, sellTimes, horizon)

	if !ok {
		return FitContext{}, nil, nil, false
	}

	adaptiveStart := horizon.Add(-probe.TradeWindow)
	buyTimes, sellTimes = splitSideEvents(ticks, adaptiveStart, horizon)
	context, ok := newFitContext(buyTimes, sellTimes, horizon)

	if !ok {
		return FitContext{}, nil, nil, false
	}

	return context, buyTimes, sellTimes, true
}

func (context FitContext) enoughEvents(buyEvents, sellEvents []time.Time) bool {
	total := len(buyEvents) + len(sellEvents)

	if total < context.MinFitEvents {
		return false
	}

	if len(buyEvents) < context.MinPerSide {
		return false
	}

	return len(sellEvents) >= context.MinPerSide
}

func gapsFromMarked(marked []markedEvent) []float64 {
	if len(marked) < 2 {
		return nil
	}

	gaps := make([]float64, 0, len(marked)-1)

	for index := 1; index < len(marked); index++ {
		gap := marked[index].at.Sub(marked[index-1].at).Seconds()

		if gap > 0 {
			gaps = append(gaps, gap)
		}
	}

	return gaps
}

func minFitEventsFor(total int) int {
	if total <= 0 {
		return bivariateParamCount * 2
	}

	identifiability := bivariateParamCount * 2
	rateScaled := int(math.Ceil(math.Sqrt(float64(total)) * math.Log(float64(total)+math.E)))

	if rateScaled < identifiability {
		return identifiability
	}

	if rateScaled > total {
		return total
	}

	return rateScaled
}

func minEventsPerSide(total int) int {
	if total <= 0 {
		return 2
	}

	perSide := int(math.Ceil(float64(total) / 4))

	if perSide < 2 {
		return 2
	}

	return perSide
}

func scanStepsFromEvents(total int) int {
	if total <= 1 {
		return 3
	}

	steps := int(math.Ceil(math.Log2(float64(total))))

	if steps < 3 {
		return 3
	}

	return steps
}

func branchFloorFor(total int) float64 {
	if total <= 0 {
		return 0
	}

	return 1 / math.Sqrt(float64(total))
}

func branchCeilingFor(total int) float64 {
	margin := 1 / math.Sqrt(float64(total))

	if margin >= criticalBranch {
		return criticalBranch / 2
	}

	return criticalBranch - margin
}

func branchScanSteps(total int) int {
	base := scanStepsFromEvents(total)
	ratio := float64(total) / float64(bivariateParamCount)

	if ratio <= float64(base) {
		return base
	}

	steps := int(math.Ceil(math.Sqrt(float64(base))))

	if steps < 3 {
		return 3
	}

	return steps
}

func selfBranchShare(total, buyCount, sellCount int) float64 {
	if total <= 0 {
		return 0
	}

	minorSide := float64(buyCount)

	if sellCount < buyCount {
		minorSide = float64(sellCount)
	}

	balance := minorSide / float64(total)

	return balance + (1-balance)/math.Sqrt(float64(total))
}

func crossBranchShare(diagonalBranch, ceiling float64) float64 {
	headroom := ceiling - diagonalBranch

	if headroom <= 0 {
		return 0
	}

	return headroom
}

func tradeWindowDuration(medianGapSec float64, totalEvents, minFitEvents int) time.Duration {
	if medianGapSec <= 0 || minFitEvents <= 0 {
		return 0
	}

	memoryFactor := math.Log(float64(totalEvents) + math.E)

	return time.Duration(medianGapSec * memoryFactor * float64(minFitEvents) * float64(time.Second))
}

func muUncertaintyFactors(count, steps int) []float64 {
	if count <= 0 {
		return []float64{1}
	}

	spread := 2 / math.Sqrt(float64(count))

	return numeric.LinSpace(1-spread, 1+spread, steps)
}

func localScaleRange(gapCV float64) (minScale, maxScale float64) {
	if gapCV <= 0 {
		return 1 - 1/math.Sqrt(8), 1 + 1/math.Sqrt(8)
	}

	minScale = 1 - gapCV

	if minScale <= 0 {
		minScale = 1 / (1 + gapCV)
	}

	maxScale = 1 + gapCV

	return minScale, maxScale
}

func confidenceHistoryCap(minFitEvents int) int {
	if minFitEvents <= 0 {
		return bivariateParamCount * 4
	}

	return minFitEvents * 4
}

func splitSideEvents(
	ticks []trade.Data,
	windowStart, windowEnd time.Time,
) ([]time.Time, []time.Time) {
	buyTimes := make([]time.Time, 0, len(ticks))
	sellTimes := make([]time.Time, 0, len(ticks))

	for _, tick := range ticks {
		if tick.Timestamp.Before(windowStart) {
			continue
		}

		if tick.Timestamp.After(windowEnd) {
			continue
		}

		switch tick.Side {
		case "buy":
			buyTimes = append(buyTimes, tick.Timestamp)
		case "sell":
			sellTimes = append(sellTimes, tick.Timestamp)
		}
	}

	return buyTimes, sellTimes
}
