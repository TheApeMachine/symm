package fluid

import (
	"fmt"
	"math"
	"time"

	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
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
	prevObservedRho              []float64
	remappedRho                  []float64

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

	return newFluidGrid(symbolConfig.tickSizeFallback, symbolConfig.gridHalfWidth, symbolConfig.integrationInterval)
}

func newFluidGrid(
	tickSize float64,
	halfWidth int,
	integrationInterval time.Duration,
) (*FluidGrid, error) {
	if tickSize <= 0 {
		return nil, fmt.Errorf("fluid: grid tick size must be positive")
	}

	if halfWidth <= 0 {
		return nil, fmt.Errorf("fluid: grid half width must be positive")
	}

	if integrationInterval <= 0 {
		return nil, fmt.Errorf("fluid: grid integration interval must be positive")
	}

	cellCount := halfWidth*2 + 1

	return &FluidGrid{
		tickSize:                     tickSize,
		halfWidth:                    halfWidth,
		midIndex:                     halfWidth,
		integrationInterval:          integrationInterval,
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
		prevObservedRho:              make([]float64, cellCount),
		remappedRho:                  make([]float64, cellCount),
	}, nil
}

func (grid *FluidGrid) ready() bool {
	return grid.stepCount >= 1
}

func (grid *FluidGrid) ingestBook(
	bids, asks []krakenmarket.BookLevel,
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
		copy(grid.rho, grid.observedRho)
		copy(grid.prevObservedRho, grid.observedRho)
		grid.lastIntegrateAt = at
		grid.prevMidPrice = midPrice

		return nil
	}

	for !at.Before(grid.lastIntegrateAt.Add(grid.integrationInterval)) {
		dt := grid.integrationInterval.Seconds()

		grid.prepareSourcesForIntegration()
		grid.inferVelocityField(midPrice, dt)
		grid.diffusionCoeff = grid.estimateDiffusionCoefficient()
		grid.integrateRK2(dt)
		grid.measureReplenishment(dt, touchSpread)
		grid.measureMidDivergence()

		grid.lastIntegrateAt = grid.lastIntegrateAt.Add(grid.integrationInterval)
		grid.prevMidPrice = midPrice
		grid.stepCount++

		grid.clearReactionAccumulators()
	}

	return nil
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
	bids, asks []krakenmarket.BookLevel,
	midPrice float64,
) {
	grid.clearField(grid.observedRho)

	for _, level := range bids {
		index := grid.priceIndex(midPrice, level.Price)

		if index < 0 {
			continue
		}

		grid.observedRho[index] += level.Qty
	}

	for _, level := range asks {
		index := grid.priceIndex(midPrice, level.Price)

		if index < 0 {
			continue
		}

		grid.observedRho[index] += level.Qty
	}
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
	touchBand := numeric.TouchBandCells(spread, grid.tickSize, grid.halfWidth)
	invInterval := 1.0 / grid.integrationInterval.Seconds()

	grid.midAddRate = grid.addAccumulator[grid.midIndex] * invInterval
	grid.midExecuteRate = grid.attributedExecuteAccumulator[grid.midIndex] * invInterval

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

func (grid *FluidGrid) measureMidDivergence() {
	index := grid.midIndex

	if index <= 0 || index >= len(grid.rho)-1 {
		grid.midDivergence = 0

		return
	}

	momentumRight := grid.rho[index+1] * grid.velocity[index+1]
	momentumLeft := grid.rho[index-1] * grid.velocity[index-1]

	grid.midDivergence = (momentumRight - momentumLeft) / (2 * grid.tickSize)
}

func (grid *FluidGrid) midVelocityDivergence() float64 {
	return grid.midDivergence
}

func (grid *FluidGrid) viscosity() float64 {
	return grid.replenishmentRate
}

func touchSpreadFromBook(bids, asks []krakenmarket.BookLevel) float64 {
	if len(bids) == 0 || len(asks) == 0 {
		return 0
	}

	return asks[0].Price - bids[0].Price
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

	total := 0.0

	for _, value := range grid.observedRho {
		if value > 0 {
			total += value
		}
	}

	if total <= 0 {
		return 0
	}

	return total / float64(len(grid.observedRho))
}

func (grid *FluidGrid) reynolds(spread float64) float64 {
	if spread <= 0 {
		return math.NaN()
	}

	if grid.replenishmentRate <= 0 {
		return math.Inf(1)
	}

	return math.Abs(grid.midPriceVelocity) * spread / grid.replenishmentRate
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
