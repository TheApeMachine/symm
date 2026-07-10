package fluid

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken"
)

/*
FluidGrid is a 1D finite-volume LOB hydrodynamics solver.

It integrates ∂ρ/∂t + ∇·(ρv) = λ_add − λ_cancel − λ_execute with Rusanov fluxes
and RK2 on a fixed exchange-time lattice. Touch divergence is ∇·(ρv).
*/
type FluidGrid struct {
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
	stepCount         int
}

/*
NewFluidGrid builds the solver from signals.fluid configuration.
*/
func NewFluidGrid() (*FluidGrid, error) {
	symbolConfig, configErr := loadSymbolConfig()

	if configErr != nil {
		return nil, configErr
	}

	return newFluidGrid(
		symbolConfig.tickSizeFallback,
		symbolConfig.gridHalfWidth,
		symbolConfig.integrationInterval,
		symbolConfig.idleThreshold,
		symbolConfig.maxIntegrationSteps,
	)
}

func newFluidGrid(
	tickSize float64,
	halfWidth int,
	integrationInterval time.Duration,
	idleThreshold time.Duration,
	maxIntegrationSteps int,
) (*FluidGrid, error) {
	if tickSize <= 0 {
		return nil, fmt.Errorf("fluid: grid tick size must be positive")
	}

	if halfWidth <= 0 {
		return nil, fmt.Errorf("fluid: grid half width must be positive")
	}

	if halfWidth > (math.MaxInt-1)/2 {
		return nil, fmt.Errorf("fluid: grid half width out of int range")
	}

	cellCount := halfWidth*2 + 1

	if cellCount <= 0 {
		return nil, fmt.Errorf("fluid: grid cell count invalid")
	}

	maxCells := maxGridCellCount(configuredBookDepthLevels())

	if maxCells > 0 && cellCount > maxCells {
		return nil, fmt.Errorf("fluid: grid cell count exceeds subscribed book depth")
	}

	if integrationInterval <= 0 {
		return nil, fmt.Errorf("fluid: grid integration interval must be positive")
	}

	if idleThreshold <= 0 {
		idleThreshold = integrationInterval * maxIntegrationStepsFloor
	}

	if maxIntegrationSteps <= 0 {
		maxIntegrationSteps = maxIntegrationStepsFloor
	}

	return &FluidGrid{
		tickSize:                     tickSize,
		halfWidth:                    halfWidth,
		midIndex:                     halfWidth,
		integrationInterval:          integrationInterval,
		idleThreshold:                idleThreshold,
		maxIntegrationSteps:          maxIntegrationSteps,
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

func (grid *FluidGrid) ready() bool {
	return grid.stepCount >= 1
}

func (grid *FluidGrid) steps() int {
	return grid.stepCount
}

func (grid *FluidGrid) ingestBook(
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

	stepsRun := 0
	for !at.Before(grid.lastIntegrateAt.Add(grid.integrationInterval)) {
		if stepsRun >= grid.maxIntegrationSteps {
			grid.lastIntegrateAt = at
			grid.prevMidPrice = midPrice
			grid.clearReactionAccumulators()
			break
		}

		dt := grid.integrationInterval.Seconds()

		grid.prepareSourcesForIntegration()
		grid.inferVelocityField(midPrice, dt)
		grid.diffusionCoeff = grid.estimateDiffusionCoefficient()

		maxV := 0.0

		for _, velocity := range grid.velocity {
			absV := math.Abs(velocity)

			if absV > maxV {
				maxV = absV
			}
		}

		maxSubDt := 1.0

		if maxV > 1e-8 {
			maxSubDt = 0.5 * grid.tickSize / maxV
		}

		if grid.diffusionCoeff > 1e-8 {
			maxSubDtDiff := 0.25 * grid.tickSize * grid.tickSize / grid.diffusionCoeff

			if maxSubDtDiff < maxSubDt {
				maxSubDt = maxSubDtDiff
			}
		}

		if maxSubDt < 1e-6 {
			maxSubDt = 1e-6
		}

		remainingDt := dt

		for remainingDt > 0 {
			subDt := math.Min(remainingDt, maxSubDt)
			grid.integrateRK2(subDt)
			remainingDt -= subDt
		}

		grid.measureReplenishment(dt, touchSpread)
		grid.measureMidDivergence()

		grid.lastIntegrateAt = grid.lastIntegrateAt.Add(grid.integrationInterval)
		grid.prevMidPrice = midPrice
		grid.stepCount++
		stepsRun++

		grid.clearReactionAccumulators()
	}

	copy(grid.prevObservedRho, grid.observedRho)

	return nil
}

func (grid *FluidGrid) resetToCurrentBook(at time.Time, midPrice float64) {
	copy(grid.rho, grid.filteredObservedRho)
	copy(grid.prevObservedRho, grid.observedRho)
	copy(grid.remappedRho, grid.filteredObservedRho)
	grid.lastIntegrateAt = at
	grid.prevMidPrice = midPrice
	grid.clearReactionAccumulators()
}

func (grid *FluidGrid) ingestTrade(
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

func (grid *FluidGrid) clearField(field []float64) {
	for index := range field {
		field[index] = 0
	}
}

func (grid *FluidGrid) projectObserved(
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

func (grid *FluidGrid) filterSparseDensity(density []float64) {
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

func (grid *FluidGrid) priceIndex(midPrice, price float64) int {
	offset := int(math.Round((price - midPrice) / grid.tickSize))
	index := grid.midIndex + offset

	if index < 0 || index >= len(grid.rho) {
		return -1
	}

	return index
}

func (grid *FluidGrid) prepareSourcesForIntegration() {
	invInterval := 1.0 / grid.integrationInterval.Seconds()

	for index := range grid.sources {
		grid.sources[index] = grid.sourceAccumulator[index] * invInterval
	}
}

func (grid *FluidGrid) measureReplenishment(dt float64, spread float64) {
	replenished := 0.0
	consumed := 0.0
	touchBand := touchBandCells(spread, grid.tickSize, grid.halfWidth)

	grid.midAddRate, grid.midExecuteRate = grid.touchBandActivityRates(spread)

	for index, rate := range grid.sources {
		if rate > 0 {
			if absInt(index-grid.midIndex) <= touchBand {
				replenished += rate
			}

			continue
		}

		if rate < 0 && absInt(index-grid.midIndex) <= touchBand {
			consumed += -rate
		}
	}

	if consumed > 0 {
		grid.replenishmentRate = replenished / consumed

		return
	}

	if replenished > 0 {
		grid.replenishmentRate = replenished * dt

		return
	}

	grid.replenishmentRate = 0
}

func (grid *FluidGrid) midAddRateAtTouch() float64 {
	return grid.midAddRate
}

func (grid *FluidGrid) midExecuteRateAtTouch() float64 {
	return grid.midExecuteRate
}

func (grid *FluidGrid) touchBandActivityRates(spread float64) (addRate, executeRate float64) {
	touchBand := touchBandCells(spread, grid.tickSize, grid.halfWidth)
	invInterval := 1.0 / grid.integrationInterval.Seconds()

	for cellIndex := range grid.addAccumulator {
		if absInt(cellIndex-grid.midIndex) > touchBand {
			continue
		}

		addRate += grid.addAccumulator[cellIndex] * invInterval
		executeRate += grid.attributedExecuteAccumulator[cellIndex] * invInterval
	}

	return addRate, executeRate
}

func (grid *FluidGrid) measureMidDivergence() {
	index := grid.midIndex

	if index <= 0 || index >= len(grid.rho)-1 {
		grid.midDivergence = 0

		return
	}

	touchDensity := math.Max(grid.observedRho[index], rhoFloor)

	grid.midDivergence = math.Abs(grid.observedRho[index]-grid.remappedRho[index]) / touchDensity
}

func (grid *FluidGrid) midVelocityDivergence() float64 {
	return grid.midDivergence
}

func (grid *FluidGrid) viscosity() float64 {
	return grid.replenishmentRate
}

func touchSpreadFromBook(bids, asks []kraken.BookLevel) float64 {
	if len(bids) == 0 || len(asks) == 0 {
		return 0
	}

	return asks[0].Price.Float64() - bids[0].Price.Float64()
}

func (grid *FluidGrid) rhoGradFloor(index int) float64 {
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

func (grid *FluidGrid) medianObservedRho() float64 {
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

func (grid *FluidGrid) reynolds(spread float64) float64 {
	return grid.reynoldsAgainst(spread, grid.replenishmentRate)
}

func (grid *FluidGrid) reynoldsAgainst(spread, viscosity float64) float64 {
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
midVorticity is |d²v/dx²| at the touch — the rotational stress proxy on the 1D book lattice.
*/
func (grid *FluidGrid) midVorticity() float64 {
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
func (grid *FluidGrid) turbulenceIntensity() float64 {
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
