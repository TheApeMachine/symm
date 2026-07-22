package fluid

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken"
)

/*
Grid is a 1D finite-volume LOB hydrodynamics solver.

It integrates ∂ρ/∂t + ∇·(ρv) = λ_add − λ_cancel − λ_execute with Rusanov fluxes
and RK2 on a fixed exchange-time lattice. Touch divergence is ∇·(ρv).
*/
type Grid struct {
	tickSize            float64
	halfWidth           int
	midIndex            int
	integrationInterval time.Duration
	idleThreshold       time.Duration
	maxIntegrationSteps int

	rho                          []float64
	velocity                     []float64
	rhoStage                     []float64
	rhoK1                        []float64
	rhoK2                        []float64
	sources                      []float64
	sourceAccumulator            []float64
	addAccumulator               []float64
	cancelAccumulator            []float64
	tradeExecuteAccumulator      []float64
	attributedExecuteAccumulator []float64
	observedRho                  []float64
	filteredObservedRho          []float64
	prevObservedRho              []float64
	remappedRho                  []float64
	filterScratch                []float64

	diffusionCoeff    float64
	lastBookAt        time.Time
	lastIntegrateAt   time.Time
	prevMidPrice      float64
	lastMidPrice      float64
	midPriceVelocity  float64
	replenishmentRate float64
	midDivergence     float64
	midAddRate        float64
	midExecuteRate    float64
	sourceBalance     float64
	stepCount         int
	lastSubsteps      int
}

/*
NewGrid builds the solver from signals.fluid configuration.
*/
func NewGrid() (*Grid, error) {
	symbolConfig, err := loadSymbolConfig()

	if err != nil {
		return nil, err
	}

	return newGrid(
		symbolConfig.tickSizeFallback,
		symbolConfig.gridHalfWidth,
		symbolConfig,
	)
}

/*
newGrid builds a lattice from already resolved symbol configuration so live
exchange metadata is not discarded by rereading global fallback settings.
*/
func newGrid(
	tickSize float64,
	halfWidth int,
	symbolConfig symbolConfig,
) (*Grid, error) {
	if tickSize <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "fluid: grid tick size must be positive", nil,
		))
	}

	if halfWidth <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "fluid: grid half width must be positive", nil,
		))
	}

	if halfWidth > (math.MaxInt-1)/2 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "fluid: grid half width out of int range", nil,
		))
	}

	cellCount := halfWidth*2 + 1

	if cellCount <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "fluid: grid cell count invalid", nil,
		))
	}

	if symbolConfig.integrationInterval <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "fluid: grid integration interval must be positive", nil,
		))
	}

	if symbolConfig.idleThreshold <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "fluid: grid idle threshold must be positive", nil,
		))
	}

	if symbolConfig.maxIntegrationSteps <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "fluid: grid integration step limit must be positive", nil,
		))
	}

	return &Grid{
		tickSize:                     tickSize,
		halfWidth:                    halfWidth,
		midIndex:                     halfWidth,
		integrationInterval:          symbolConfig.integrationInterval,
		idleThreshold:                symbolConfig.idleThreshold,
		maxIntegrationSteps:          symbolConfig.maxIntegrationSteps,
		rho:                          make([]float64, cellCount),
		velocity:                     make([]float64, cellCount),
		rhoStage:                     make([]float64, cellCount),
		rhoK1:                        make([]float64, cellCount),
		rhoK2:                        make([]float64, cellCount),
		sources:                      make([]float64, cellCount),
		sourceAccumulator:            make([]float64, cellCount),
		addAccumulator:               make([]float64, cellCount),
		cancelAccumulator:            make([]float64, cellCount),
		tradeExecuteAccumulator:      make([]float64, cellCount),
		attributedExecuteAccumulator: make([]float64, cellCount),
		observedRho:                  make([]float64, cellCount),
		filteredObservedRho:          make([]float64, cellCount),
		prevObservedRho:              make([]float64, cellCount),
		remappedRho:                  make([]float64, cellCount),
		filterScratch:                make([]float64, cellCount),
	}, nil
}

/*
ready reports whether the grid has enough integrated state to publish a fluid
reading.
*/
func (grid *Grid) ready() bool {
	return grid.stepCount >= 1
}

/*
steps returns completed grid integrations so measurement maturity reflects
actual observations.
*/
func (grid *Grid) steps() int {
	return grid.stepCount
}

/*
ingestBook applies an order-book update to density and source fields so the
grid represents current resting liquidity.
*/
func (grid *Grid) ingestBook(
	bids, asks []kraken.BookLevel,
	midPrice float64,
	at time.Time,
) error {
	if midPrice <= 0 {
		return fmt.Errorf("fluid: grid mid price must be positive")
	}

	if at.IsZero() {
		return fmt.Errorf("fluid: grid book event time must be set")
	}

	grid.projectObserved(bids, asks, midPrice)
	touchSpread := touchSpreadFromBook(bids, asks)

	if !grid.lastBookAt.IsZero() {
		if !at.After(grid.lastBookAt) {
			return nil
		}

		grid.accumulateReactionSources(midPrice)
	}

	grid.lastBookAt = at
	grid.lastMidPrice = midPrice

	if grid.lastIntegrateAt.IsZero() {
		grid.resetToCurrentBook(at, midPrice)

		return nil
	}

	if grid.idleThreshold > 0 && at.Sub(grid.lastIntegrateAt) > grid.idleThreshold {
		grid.resetToCurrentBook(at, midPrice)

		return nil
	}

	elapsed := at.Sub(grid.lastIntegrateAt)

	if elapsed < grid.integrationInterval {
		copy(grid.prevObservedRho, grid.observedRho)

		return nil
	}

	steps := int(math.Ceil(float64(elapsed) / float64(grid.integrationInterval)))

	if steps > grid.maxIntegrationSteps {
		steps = grid.maxIntegrationSteps
	}

	observationSeconds := elapsed.Seconds()
	dt := observationSeconds / float64(steps)
	grid.prepareSourcesForIntegration(observationSeconds)
	grid.inferVelocityField(midPrice, observationSeconds)
	grid.diffusionCoeff = grid.estimateDiffusionCoefficient()

	for range steps {
		if err := grid.integrateInterval(dt); err != nil {
			return err
		}

		grid.stepCount++
	}

	grid.measureReplenishment(touchSpread, observationSeconds)
	grid.measureMidDivergence()
	grid.lastIntegrateAt = at
	grid.prevMidPrice = midPrice
	grid.clearReactionAccumulators()

	copy(grid.prevObservedRho, grid.observedRho)

	return nil
}

/*
resetToCurrentBook reinitializes density from the current book after
discontinuity so stale solver state cannot contaminate a new epoch.
*/
func (grid *Grid) resetToCurrentBook(at time.Time, midPrice float64) {
	copy(grid.rho, grid.filteredObservedRho)
	copy(grid.prevObservedRho, grid.observedRho)
	copy(grid.remappedRho, grid.filteredObservedRho)
	grid.lastIntegrateAt = at
	grid.prevMidPrice = midPrice
	grid.clearReactionAccumulators()
}

/*
ingestTrade applies executed flow to the grid so observed removals affect
velocity and reaction sources.
*/
func (grid *Grid) ingestTrade(
	tradePrice, qty float64,
	at time.Time,
) error {
	if tradePrice <= 0 {
		return fmt.Errorf("fluid: trade price must be positive")
	}

	if qty <= 0 {
		return fmt.Errorf("fluid: trade quantity must be positive")
	}

	if at.IsZero() {
		return fmt.Errorf("fluid: trade event time must be set")
	}

	if grid.lastMidPrice <= 0 {
		return fmt.Errorf("fluid: trade arrived before book mid price was set")
	}

	index := grid.priceIndex(grid.lastMidPrice, tradePrice)

	if index < 0 {
		return fmt.Errorf("fluid: trade price %v outside grid span", tradePrice)
	}

	grid.tradeExecuteAccumulator[index] += qty

	return nil
}

/*
clearField zeros a reusable grid field so the next integration step does not
retain prior transient values.
*/
func (grid *Grid) clearField(field []float64) {
	for index := range field {
		field[index] = 0
	}
}

/*
projectObserved projects observed book density onto the grid so empirical
levels anchor solver state.
*/
func (grid *Grid) projectObserved(
	bids, asks []kraken.BookLevel,
	midPrice float64,
) {
	grid.clearField(grid.observedRho)

	for _, level := range bids {
		index := grid.priceIndex(midPrice, level.Price.Float64())

		if index < 0 {
			continue
		}

		grid.observedRho[index] += level.Qty
	}

	for _, level := range asks {
		index := grid.priceIndex(midPrice, level.Price.Float64())

		if index < 0 {
			continue
		}

		grid.observedRho[index] += level.Qty
	}

	copy(grid.filteredObservedRho, grid.observedRho)
	grid.filterSparseDensity(grid.filteredObservedRho)
}

/*
filterSparseDensity removes unsupported isolated density cells so numerical
artifacts are not treated as liquidity.
*/
func (grid *Grid) filterSparseDensity(density []float64) {
	if len(density) < 3 || len(grid.filterScratch) != len(density) {
		return
	}

	total := densityMass(density)
	if total <= 0 {
		return
	}

	copy(grid.filterScratch, density)

	for index := 1; index < len(density)-1; index++ {
		left := positiveFinite(density[index-1])
		center := positiveFinite(density[index])
		right := positiveFinite(density[index+1])
		localMass := left + center + right

		if localMass <= rhoFloor {
			continue
		}

		curvature := math.Abs(left - 2*center + right)
		alpha := curvature / (curvature + localMass + rhoFloor)
		target := localMass / 3
		delta := alpha * (target - center)

		grid.filterScratch[index] += delta
		grid.filterScratch[index-1] -= delta / 2
		grid.filterScratch[index+1] -= delta / 2
	}

	filteredTotal := 0.0
	for index, value := range grid.filterScratch {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			value = 0
		}

		grid.filterScratch[index] = value
		filteredTotal += value
	}

	if filteredTotal <= 0 {
		return
	}

	scale := total / filteredTotal
	for index := range density {
		density[index] = grid.filterScratch[index] * scale
	}
}

func densityMass(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += positiveFinite(value)
	}

	return total
}

func positiveFinite(value float64) float64 {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	return value
}

/*
priceIndex maps a market price to its grid cell so book and trade events share
one spatial coordinate system.
*/
func (grid *Grid) priceIndex(midPrice, price float64) int {
	offset := int(math.Round((price - midPrice) / grid.tickSize))
	index := grid.midIndex + offset

	if index < 0 || index >= len(grid.rho) {
		return -1
	}

	return index
}

/*
prepareSourcesForIntegration finalizes accumulated source terms so each solver
step consumes one coherent interval.
*/
func (grid *Grid) prepareSourcesForIntegration(observationSeconds float64) {
	invInterval := 1.0 / observationSeconds

	for index := range grid.sourceAccumulator {
		grid.sources[index] = grid.sourceAccumulator[index] * invInterval
	}
}

/*
measureReplenishment derives touch replenishment rates from observed source
changes so liquidity recovery remains data-driven.
*/
func (grid *Grid) measureReplenishment(spread, observationSeconds float64) {
	replenished := 0.0
	consumed := 0.0
	touchBand := touchBandCells(spread, grid.tickSize, grid.halfWidth)

	grid.midAddRate, grid.midExecuteRate = grid.touchBandActivityRates(
		spread,
		observationSeconds,
	)
	grid.sourceBalance = 0

	invObservation := 1.0 / observationSeconds

	for index := range grid.sourceAccumulator {
		if absInt(index-grid.midIndex) <= touchBand {
			grid.sourceBalance += grid.sourceAccumulator[index]
			replenished += grid.addAccumulator[index] * invObservation
			consumed += (grid.cancelAccumulator[index] +
				grid.attributedExecuteAccumulator[index]) * invObservation
		}
	}

	if consumed > 0 {
		grid.replenishmentRate = replenished / consumed

		return
	}

	grid.replenishmentRate = 0
}

/*
midAddRateAtTouch returns the current near-touch addition rate published as
fluid evidence.
*/
func (grid *Grid) midAddRateAtTouch() float64 {
	return grid.midAddRate
}

/*
midExecuteRateAtTouch returns the current near-touch execution rate published
as fluid evidence.
*/
func (grid *Grid) midExecuteRateAtTouch() float64 {
	return grid.midExecuteRate
}

/*
touchBandActivityRates aggregates addition and execution inside the observed
spread band so touch activity uses the instrument's scale.
*/
func (grid *Grid) touchBandActivityRates(
	spread,
	observationSeconds float64,
) (addRate, executeRate float64) {
	touchBand := touchBandCells(spread, grid.tickSize, grid.halfWidth)
	invInterval := 1.0 / observationSeconds

	for cellIndex := range grid.addAccumulator {
		if absInt(cellIndex-grid.midIndex) > touchBand {
			continue
		}

		addRate += grid.addAccumulator[cellIndex] * invInterval
		executeRate += grid.attributedExecuteAccumulator[cellIndex] * invInterval
	}

	return addRate, executeRate
}

/*
measureMidDivergence evaluates signed ∇·(ρv) at the empty midpoint using
the observed resting-density median as its normalization scale. It uses the
same Rusanov face fluxes as the RK2 solver, preserving the solver's direction
without dividing by the structurally empty cell between bid and ask.
*/
func (grid *Grid) measureMidDivergence() {
	index := grid.midIndex

	if index <= 0 || index >= len(grid.rho)-1 {
		grid.midDivergence = 0

		return
	}

	touchDensity := math.Max(
		grid.rho[index],
		math.Max(grid.medianObservedRho(), rhoFloor),
	)

	grid.midDivergence = grid.faceFluxDivergence(grid.rho, index) / touchDensity
}

/*
midVelocityDivergence calculates velocity divergence around the midpoint so
local compression and expansion remain spatially grounded.
*/
func (grid *Grid) midVelocityDivergence() float64 {
	return grid.midDivergence
}

/*
viscosity derives effective market viscosity from observed density and
velocity gradients so resistance is not fixed.
*/
func (grid *Grid) viscosity() float64 {
	return grid.replenishmentRate
}

func touchSpreadFromBook(bids, asks []kraken.BookLevel) float64 {
	if len(bids) == 0 || len(asks) == 0 {
		return 0
	}

	return asks[0].Price.Float64() - bids[0].Price.Float64()
}

/*
rhoGradFloor derives a local density-gradient floor so viscosity remains tied
to observable book structure.
*/
func (grid *Grid) rhoGradFloor(index int) float64 {
	if index < 0 || index >= len(grid.observedRho) {
		return rhoFloor
	}

	medianRho := grid.medianObservedRho()

	if medianRho <= 0 {
		return rhoFloor
	}

	local := grid.observedRho[index]

	if local <= 0 {
		local = medianRho
	}

	floor := local * local / (local + medianRho)

	if floor < rhoFloor {
		return rhoFloor
	}

	return floor
}

/*
medianObservedRho returns median observed density so sparse cells do not
dictate the grid's reference scale.
*/
func (grid *Grid) medianObservedRho() float64 {
	if len(grid.observedRho) == 0 {
		return 0
	}

	positive := make([]float64, 0, len(grid.observedRho))

	for _, value := range grid.observedRho {
		if value > 0 {
			positive = append(positive, value)
		}
	}

	median, ok := statistic.MedianOf(positive)
	if !ok {
		return 0
	}

	return median
}

/*
reynolds calculates the current market Reynolds quantity from observed spread
and derived viscosity.
*/
func (grid *Grid) reynolds(spread float64) float64 {
	return grid.reynoldsAgainst(spread, grid.replenishmentRate)
}

/*
reynoldsAgainst calculates Reynolds against explicit viscosity so callers can
compare compatible fluid regimes.
*/
func (grid *Grid) reynoldsAgainst(spread, viscosity float64) float64 {
	if spread <= 0 {
		return math.NaN()
	}

	index := grid.midIndex
	flow := math.Abs(grid.midPriceVelocity)
	touchBand := touchBandCells(spread, grid.tickSize, grid.halfWidth)

	if flow <= 0 {
		flow = grid.midAddRate + grid.midExecuteRate
	}

	if flow <= 0 {
		touchChange := 0.0

		for cellIndex := range grid.observedRho {
			if absInt(cellIndex-index) > touchBand {
				continue
			}

			touchChange += math.Abs(grid.observedRho[cellIndex] - grid.remappedRho[cellIndex])
		}

		flow = touchChange / grid.integrationInterval.Seconds()
	}

	if flow <= 0 {
		return 0
	}

	if viscosity <= 0 {
		return math.Inf(1)
	}

	return flow * spread / viscosity
}

/*
midVelocityCurvature is |d²v/dx²| at the touch on the 1D book lattice.
*/
func (grid *Grid) midVelocityCurvature() float64 {
	index := grid.midIndex

	if index <= 0 || index >= len(grid.velocity)-1 {
		return 0
	}

	denominator := grid.tickSize * grid.tickSize

	if denominator <= 0 {
		return 0
	}

	laplacian := grid.velocity[index+1] - 2*grid.velocity[index] + grid.velocity[index-1]

	return math.Abs(laplacian) / denominator
}

/*
turbulenceIntensity is the RMS velocity fluctuation across the projected book field.
*/
func (grid *Grid) turbulenceIntensity() float64 {
	cellCount := len(grid.velocity)

	if cellCount == 0 {
		return 0
	}

	mean := 0.0

	for _, velocity := range grid.velocity {
		mean += velocity
	}

	mean /= float64(cellCount)

	variance := 0.0

	for _, velocity := range grid.velocity {
		delta := velocity - mean
		variance += delta * delta
	}

	variance /= float64(cellCount)

	return math.Sqrt(variance)
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}

	return value
}
