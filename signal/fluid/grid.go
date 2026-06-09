package fluid

import (
	"fmt"
	"math"
	"time"

	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

const (
	laminarReynoldsCeiling = 2000
	turbulentReynoldsFloor = 4000
	touchBandFraction      = 0.25
)

/*
FluidGrid is a 1D finite-volume LOB hydrodynamics solver.

It integrates ∂ρ/∂t + ∇·(ρv) = λ_add − λ_cancel − λ_execute with Rusanov fluxes
and RK2 on a fixed exchange-time lattice.
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
	stepCount         int
}

/*
NewFluidGrid builds the solver from signals.fluid configuration.
*/
func NewFluidGrid() (*FluidGrid, error) {
	tickSize := viper.GetFloat64("signals.fluid.tick_size")

	if tickSize <= 0 {
		return nil, fmt.Errorf("signals.fluid.tick_size must be positive")
	}

	halfWidth := viper.GetInt("signals.fluid.grid_half_width")

	if halfWidth <= 0 {
		return nil, fmt.Errorf("signals.fluid.grid_half_width must be positive")
	}

	integrationInterval := viper.GetDuration("signals.fluid.integration_interval")

	if integrationInterval <= 0 {
		return nil, fmt.Errorf("signals.fluid.integration_interval must be positive")
	}

	return newFluidGrid(tickSize, halfWidth, integrationInterval)
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

	if !grid.lastBookAt.IsZero() {
		if !at.After(grid.lastBookAt) {
			return fmt.Errorf("fluid: book timestamps must be strictly increasing")
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
		grid.measureReplenishment(dt)
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

func (grid *FluidGrid) measureReplenishment(dt float64) {
	positive := 0.0
	touchBand := int(float64(grid.halfWidth) * touchBandFraction)

	if touchBand < 1 {
		touchBand = 1
	}

	for index, rate := range grid.sources {
		if rate <= 0 {
			continue
		}

		if absInt(index-grid.midIndex) > touchBand {
			continue
		}

		positive += rate
	}

	grid.replenishmentRate = positive * dt
}

func (grid *FluidGrid) measureMidDivergence() {
	index := grid.midIndex

	if index <= 0 || index >= len(grid.velocity)-1 {
		grid.midDivergence = 0

		return
	}

	grid.midDivergence = (grid.velocity[index+1] - grid.velocity[index-1]) /
		(2 * grid.tickSize)
}

func (grid *FluidGrid) midVelocityDivergence() float64 {
	return grid.midDivergence
}

func (grid *FluidGrid) viscosity() float64 {
	return grid.replenishmentRate
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

func absInt(value int) int {
	if value < 0 {
		return -value
	}

	return value
}
