package hawkes

import (
	"math"
	"time"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
)

const bivariateParamCount = 7

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

type arrivalTune struct {
	totalEvents int
	buyEvents   int
	sellEvents  int
}

/*
NewFitContext derives fit bounds from observed trade arrivals.
*/
func NewFitContext(stream ArrivalStream, horizon time.Time) (FitContext, bool) {
	marked := stream.Marked()

	if len(marked) < 2 {
		return FitContext{}, false
	}

	span := stream.Span(horizon)

	if span <= 0 {
		return FitContext{}, false
	}

	gaps := stream.Gaps()

	if len(gaps) == 0 {
		return FitContext{}, false
	}

	medianGap := numeric.Median(gaps)
	lowerGap, upperGap := numeric.Quartiles(gaps)

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
	tune := arrivalTune{
		totalEvents: len(stream.buy.Times()) + len(stream.sell.Times()),
		buyEvents:   len(stream.buy.Times()),
		sellEvents:  len(stream.sell.Times()),
	}
	localMin, localMax := tune.localScaleRange(gapCV)

	return FitContext{
		SpanSec:      span,
		MedianGapSec: medianGap,
		GapLowerSec:  lowerGap,
		GapUpperSec:  upperGap,
		GapCV:        gapCV,
		TotalEvents:  tune.totalEvents,
		BuyEvents:    tune.buyEvents,
		SellEvents:   tune.sellEvents,
		MinFitEvents: tune.minFitEvents(),
		MinPerSide:   tune.minEventsPerSide(),
		TradeWindow: tune.tradeWindowDuration(
			medianGap, tune.minFitEvents(),
		),
		ScanSteps:       tune.scanSteps(),
		BranchScanSteps: tune.branchScanSteps(),
		BranchFloor:     tune.branchFloor(),
		BranchCeiling:   tune.branchCeiling(),
		BetaCandidates: numeric.LogSpace(
			1/upperGap, 1/lowerGap, tune.scanSteps(),
		),
		MuBuyFactors: tune.muUncertaintyFactors(tune.buyEvents),
		MuSellFactors: tune.muUncertaintyFactors(
			tune.sellEvents,
		),
		BranchSelfCandidates: numeric.LinSpace(
			tune.branchFloor(),
			tune.branchCeiling()*tune.selfBranchShare(),
			tune.branchScanSteps(),
		),
		BranchCrossCandidates: numeric.LinSpace(
			0, tune.branchCeiling(), tune.branchScanSteps(),
		),
		LocalScales: numeric.LinSpace(
			localMin, localMax, tune.scanSteps(),
		),
	}, true
}

/*
FitContextFromTicks builds an adaptive fit context and arrival stream from ticks.
*/
func FitContextFromTicks(
	ticks []market.TradeUpdate,
	windowStart, horizon time.Time,
) (FitContext, ArrivalStream, bool) {
	stream := ArrivalStreamFromTicks(ticks, windowStart, horizon)

	if stream.buy.Len()+stream.sell.Len() < 2 {
		return FitContext{}, ArrivalStream{}, false
	}

	probe, ok := NewFitContext(stream, horizon)

	if !ok {
		return FitContext{}, ArrivalStream{}, false
	}

	adaptiveStart := horizon.Add(-probe.TradeWindow)
	stream = ArrivalStreamFromTicks(ticks, adaptiveStart, horizon)
	context, ok := NewFitContext(stream, horizon)

	if !ok {
		return FitContext{}, ArrivalStream{}, false
	}

	return context, stream, true
}

func (context FitContext) EnoughEvents(stream ArrivalStream) bool {
	total := stream.buy.Len() + stream.sell.Len()

	if total < context.MinFitEvents {
		return false
	}

	if stream.buy.Len() < context.MinPerSide {
		return false
	}

	return stream.sell.Len() >= context.MinPerSide
}

func (context FitContext) MuBuyStart() float64 {
	muBuy := float64(context.BuyEvents) / context.SpanSec

	if muBuy <= 0 {
		return 1 / context.SpanSec
	}

	return muBuy
}

func (context FitContext) MuSellStart() float64 {
	muSell := float64(context.SellEvents) / context.SpanSec

	if muSell <= 0 {
		return 1 / context.SpanSec
	}

	return muSell
}

func (context FitContext) CrossBranchCap(diagonalBranch float64) float64 {
	headroom := context.BranchCeiling - diagonalBranch

	if headroom <= 0 {
		return 0
	}

	return headroom
}

func (tune arrivalTune) minFitEvents() int {
	if tune.totalEvents <= 0 {
		return bivariateParamCount * 2
	}

	identifiability := bivariateParamCount * 2
	rateScaled := int(
		math.Ceil(
			math.Sqrt(float64(tune.totalEvents)) *
				math.Log(float64(tune.totalEvents)+math.E),
		),
	)

	if rateScaled < identifiability {
		return identifiability
	}

	if rateScaled > tune.totalEvents {
		return tune.totalEvents
	}

	return rateScaled
}

func (tune arrivalTune) minEventsPerSide() int {
	if tune.totalEvents <= 0 {
		return 2
	}

	perSide := int(math.Ceil(float64(tune.totalEvents) / 4))

	if perSide < 2 {
		return 2
	}

	return perSide
}

func (tune arrivalTune) scanSteps() int {
	if tune.totalEvents <= 1 {
		return 3
	}

	steps := int(math.Ceil(math.Log2(float64(tune.totalEvents))))

	if steps < 3 {
		return 3
	}

	return steps
}

func (tune arrivalTune) branchFloor() float64 {
	if tune.totalEvents <= 0 {
		return 0
	}

	return 1 / math.Sqrt(float64(tune.totalEvents))
}

func (tune arrivalTune) branchCeiling() float64 {
	margin := 1 / math.Sqrt(float64(tune.totalEvents))

	if margin >= criticalBranch {
		return criticalBranch / 2
	}

	return criticalBranch - margin
}

func (tune arrivalTune) branchScanSteps() int {
	base := tune.scanSteps()
	ratio := float64(tune.totalEvents) / float64(bivariateParamCount)

	if ratio <= float64(base) {
		return base
	}

	steps := int(math.Ceil(math.Sqrt(float64(base))))

	if steps < 3 {
		return 3
	}

	return steps
}

func (tune arrivalTune) selfBranchShare() float64 {
	if tune.totalEvents <= 0 {
		return 0
	}

	minorSide := float64(tune.buyEvents)

	if tune.sellEvents < tune.buyEvents {
		minorSide = float64(tune.sellEvents)
	}

	balance := minorSide / float64(tune.totalEvents)

	return balance + (1-balance)/math.Sqrt(float64(tune.totalEvents))
}

func (tune arrivalTune) tradeWindowDuration(
	medianGapSec float64,
	minFitEvents int,
) time.Duration {
	if medianGapSec <= 0 || minFitEvents <= 0 {
		return 0
	}

	memoryFactor := math.Log(float64(tune.totalEvents) + math.E)

	return time.Duration(
		medianGapSec * memoryFactor * float64(minFitEvents) * float64(time.Second),
	)
}

func (tune arrivalTune) muUncertaintyFactors(count int) []float64 {
	if count <= 0 {
		return []float64{1}
	}

	spread := 2 / math.Sqrt(float64(count))

	return numeric.LinSpace(1-spread, 1+spread, tune.scanSteps())
}

func (tune arrivalTune) localScaleRange(gapCV float64) (minScale, maxScale float64) {
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
