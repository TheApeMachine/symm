package hawkes

import (
	"math"
	"time"
)

/*
bivariateParamCount is the number of free parameters in one bivariate
exponential-kernel Hawkes fit: muX, muY, beta, and four excitation
amplitudes.
*/
const bivariateParamCount = 7

/*
fitContext holds data-derived bounds and optimizer scan grids for one
bivariate Hawkes fit. Nothing here is a fixed constant: every bound and grid
step is a function of the observed event count and inter-arrival gap scale,
so a quiet market and a bursting one search different, appropriately scaled
parameter spaces.
*/
type fitContext struct {
	observedFromSec       float64
	throughSec            float64
	spanSec               float64
	medianGapSec          float64
	gapLowerSec           float64
	gapUpperSec           float64
	gapCV                 float64
	totalEvents           int
	eventsX               int
	eventsY               int
	minFitEvents          int
	minPerSide            int
	scanSteps             int
	branchScanSteps       int
	branchFloor           float64
	branchCeiling         float64
	tradeWindow           time.Duration
	betaCandidates        []float64
	muXFactors            []float64
	muYFactors            []float64
	branchSelfCandidates  []float64
	branchCrossCandidates []float64
	localScales           []float64
}

type arrivalTune struct {
	totalEvents int
	eventsX     int
	eventsY     int
}

/*
newFitContext derives fit bounds and optimizer scan grids from an arrival
stream observed through horizon.
*/
func newFitContext(stream arrivalStream, horizonSec float64) (fitContext, bool) {
	context, ok := newObservationContext(stream, horizonSec)

	if !ok {
		return fitContext{}, false
	}

	return context.withSearchGrid()
}

/*
newObservationContext derives the exact fit statistics needed between
refits, omitting the optimizer candidate grids (which newFitContext adds).
*/
func newObservationContext(stream arrivalStream, horizonSec float64) (fitContext, bool) {
	marked := stream.observationMarked(horizonSec)

	if len(marked) < 2 {
		return fitContext{}, false
	}

	span := stream.span(horizonSec)

	if span <= 0 {
		return fitContext{}, false
	}

	gaps := gapsFromMarked(marked)

	if len(gaps) == 0 {
		return fitContext{}, false
	}

	summary := newGapSummaryFromGaps(gaps)
	medianGap, ok := summary.median()

	if !ok || medianGap <= 0 {
		return fitContext{}, false
	}

	lowerGap, upperGap, err := summary.quartiles()

	if err != nil {
		return fitContext{}, false
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
	buyCount, sellCount := stream.observationCounts(horizonSec)
	tune := arrivalTune{
		totalEvents: buyCount + sellCount,
		eventsX:     buyCount,
		eventsY:     sellCount,
	}

	minFitEvents := tune.minFitEvents()

	return fitContext{
		observedFromSec: stream.originSec,
		throughSec:      horizonSec,
		spanSec:         span,
		medianGapSec:    medianGap,
		gapLowerSec:     lowerGap,
		gapUpperSec:     upperGap,
		gapCV:           gapCV,
		totalEvents:     tune.totalEvents,
		eventsX:         tune.eventsX,
		eventsY:         tune.eventsY,
		minFitEvents:    minFitEvents,
		minPerSide:      tune.minEventsPerSide(),
		tradeWindow:     tune.tradeWindowDuration(medianGap, minFitEvents),
		scanSteps:       tune.scanSteps(),
		branchScanSteps: tune.branchScanSteps(),
		branchFloor:     tune.branchFloor(),
		branchCeiling:   tune.branchCeiling(),
	}, true
}

func (fitContext fitContext) withSearchGrid() (fitContext, bool) {
	tune := arrivalTune{
		totalEvents: fitContext.totalEvents,
		eventsX:     fitContext.eventsX,
		eventsY:     fitContext.eventsY,
	}
	localMin, localMax := tune.localScaleRange(fitContext.gapCV)
	var err error

	fitContext.betaCandidates, err = logspace(
		1/fitContext.gapUpperSec, 1/fitContext.gapLowerSec, fitContext.scanSteps,
	)

	if err != nil {
		return fitContext{}, false
	}

	fitContext.branchSelfCandidates, err = linspace(
		fitContext.branchFloor,
		fitContext.branchCeiling*tune.selfBranchShare(),
		fitContext.branchScanSteps,
	)

	if err != nil {
		return fitContext{}, false
	}

	fitContext.branchCrossCandidates, err = linspace(
		0, fitContext.branchCeiling, fitContext.branchScanSteps,
	)

	if err != nil {
		return fitContext{}, false
	}

	fitContext.localScales, err = linspace(localMin, localMax, fitContext.scanSteps)

	if err != nil {
		return fitContext{}, false
	}

	fitContext.muXFactors, err = tune.muUncertaintyFactors(fitContext.eventsX)

	if err != nil {
		return fitContext{}, false
	}

	fitContext.muYFactors, err = tune.muUncertaintyFactors(fitContext.eventsY)

	if err != nil {
		return fitContext{}, false
	}

	return fitContext, true
}

/*
enoughEvents reports whether the stream satisfies context minima at horizon.
*/
func (fitContext fitContext) enoughEvents(stream arrivalStream) bool {
	buyCount, sellCount := stream.observationCounts(fitContext.throughSec)
	total := buyCount + sellCount

	if total < fitContext.minFitEvents {
		return false
	}

	if buyCount < fitContext.minPerSide {
		return false
	}

	return sellCount >= fitContext.minPerSide
}

/*
muXStart returns the event-rate seed for stream x.
*/
func (fitContext fitContext) muXStart() float64 {
	return float64(fitContext.eventsX) / fitContext.spanSec
}

/*
muYStart returns the event-rate seed for stream y.
*/
func (fitContext fitContext) muYStart() float64 {
	return float64(fitContext.eventsY) / fitContext.spanSec
}

/*
poissonFit returns the no-excitation bivariate baseline for this stream.
*/
func (fitContext fitContext) poissonFit() bivariateFit {
	fit := bivariateFit{
		muX:  fitContext.muXStart(),
		muY:  fitContext.muYStart(),
		beta: 1 / fitContext.medianGapSec,
	}
	fit.intensityX = fit.muX
	fit.intensityY = fit.muY

	return fit
}

/*
crossBranchCap returns the cross-excitation ceiling given a diagonal branch.
*/
func (fitContext fitContext) crossBranchCap(diagonalBranch float64) float64 {
	headroom := fitContext.branchCeiling - diagonalBranch

	if headroom <= 0 {
		return 0
	}

	return headroom
}

func (arrivalTune arrivalTune) minFitEvents() int {
	if arrivalTune.totalEvents <= 0 {
		return bivariateParamCount * 2
	}

	identifiability := bivariateParamCount * 2
	rateScaled := int(math.Ceil(
		math.Sqrt(float64(arrivalTune.totalEvents)) * math.Log(float64(arrivalTune.totalEvents)+math.E),
	))

	if rateScaled < identifiability {
		return identifiability
	}

	if rateScaled > arrivalTune.totalEvents {
		return arrivalTune.totalEvents
	}

	return rateScaled
}

func (arrivalTune arrivalTune) minEventsPerSide() int {
	if arrivalTune.totalEvents <= 0 {
		return 2
	}

	perSide := int(math.Ceil(float64(arrivalTune.totalEvents) / 4))

	if perSide < 2 {
		return 2
	}

	return perSide
}

func (arrivalTune arrivalTune) scanSteps() int {
	if arrivalTune.totalEvents <= 1 {
		return 3
	}

	steps := int(math.Ceil(math.Log2(float64(arrivalTune.totalEvents))))

	if steps < 3 {
		return 3
	}

	return steps
}

func (arrivalTune arrivalTune) branchFloor() float64 {
	if arrivalTune.totalEvents <= 0 {
		return 0
	}

	return 1 / math.Sqrt(float64(arrivalTune.totalEvents))
}

func (arrivalTune arrivalTune) branchCeiling() float64 {
	if arrivalTune.totalEvents <= 0 {
		panic("hawkes: branchCeiling requires positive event mass")
	}

	margin := 1 / math.Sqrt(float64(arrivalTune.totalEvents))

	if margin >= criticalBranch {
		return criticalBranch / 2
	}

	return criticalBranch - margin
}

func (arrivalTune arrivalTune) branchScanSteps() int {
	base := arrivalTune.scanSteps()
	ratio := float64(arrivalTune.totalEvents) / float64(bivariateParamCount)

	if ratio <= float64(base) {
		return base
	}

	steps := int(math.Ceil(math.Sqrt(float64(base))))

	if steps < 3 {
		return 3
	}

	return steps
}

func (arrivalTune arrivalTune) selfBranchShare() float64 {
	if arrivalTune.totalEvents <= 0 {
		return 0
	}

	minorSide := float64(arrivalTune.eventsX)

	if arrivalTune.eventsY < arrivalTune.eventsX {
		minorSide = float64(arrivalTune.eventsY)
	}

	balance := minorSide / float64(arrivalTune.totalEvents)

	return balance + (1-balance)/math.Sqrt(float64(arrivalTune.totalEvents))
}

func (arrivalTune arrivalTune) localScaleRange(gapCV float64) (minScale, maxScale float64) {
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

/*
tradeWindowDuration is the retention horizon the fit's own event-rate memory
implies: enough median gaps, scaled by the observed count's own log-memory
factor, to gather minFitEvents worth of history.
*/
func (arrivalTune arrivalTune) tradeWindowDuration(
	medianGapSec float64,
	minFitEvents int,
) time.Duration {
	if medianGapSec <= 0 || minFitEvents <= 0 {
		return 0
	}

	memoryFactor := math.Log(float64(arrivalTune.totalEvents) + math.E)

	return time.Duration(
		medianGapSec * memoryFactor * float64(minFitEvents) * float64(time.Second),
	)
}

/*
muUncertaintyFactors returns multiplicative perturbations around a baseline
rate, scaled by that side's own sampling uncertainty (1/sqrt(count)), for
multi-start seeding.
*/
func (arrivalTune arrivalTune) muUncertaintyFactors(count int) ([]float64, error) {
	if count <= 0 {
		return []float64{1}, nil
	}

	spread := 2 / math.Sqrt(float64(count))

	return linspace(1-spread, 1+spread, arrivalTune.scanSteps())
}
