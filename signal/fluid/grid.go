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
FluidGrid discretizes the order book into a 1D finite-difference fluid field.

Density ρ is resting quantity per tick. Velocity v is mid-price advection in
price space. Source-sink terms capture net adds, cancels, and executions
between frames per the continuity equation.
*/
type FluidGrid struct {
	tickSize          float64
	halfWidth         int
	density           []float64
	previousDensity   []float64
	velocity          []float64
	sourceSink        []float64
	midIndex          int
	lastStepAt        time.Time
	prevMidPrice      float64
	midPriceVelocity  float64
	replenishmentRate float64
	stepCount         int
}

/*
NewFluidGrid builds a grid from signals.fluid.tick_size and grid_half_width.
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

	size := halfWidth*2 + 1

	return &FluidGrid{
		tickSize:        tickSize,
		halfWidth:       halfWidth,
		density:         make([]float64, size),
		previousDensity: make([]float64, size),
		velocity:        make([]float64, size),
		sourceSink:      make([]float64, size),
		midIndex:        halfWidth,
	}, nil
}

func (grid *FluidGrid) step(
	bids, asks []krakenmarket.BookLevel,
	midPrice float64,
	at time.Time,
) error {
	if midPrice <= 0 {
		return fmt.Errorf("fluid: grid mid price must be positive")
	}

	if at.IsZero() {
		return fmt.Errorf("fluid: grid step time must be set")
	}

	copy(grid.previousDensity, grid.density)
	grid.clearField(grid.density)
	grid.clearField(grid.sourceSink)

	grid.projectBook(bids, asks, midPrice)

	elapsed := grid.elapsedSeconds(at)

	if elapsed <= 0 {
		grid.lastStepAt = at
		grid.prevMidPrice = midPrice

		return nil
	}

	grid.computeSourceSink(elapsed)
	grid.computeVelocity(elapsed, midPrice)
	grid.replenishmentRate = grid.replenishment(elapsed)
	grid.lastStepAt = at
	grid.stepCount++

	return nil
}

func (grid *FluidGrid) ready() bool {
	return grid.stepCount >= 1 && grid.lastStepAt.IsZero() == false
}

func (grid *FluidGrid) clearField(field []float64) {
	for index := range field {
		field[index] = 0
	}
}

func (grid *FluidGrid) elapsedSeconds(at time.Time) float64 {
	if grid.lastStepAt.IsZero() {
		return 0
	}

	return at.Sub(grid.lastStepAt).Seconds()
}

func (grid *FluidGrid) projectBook(
	bids, asks []krakenmarket.BookLevel,
	midPrice float64,
) {
	for _, level := range bids {
		index := grid.priceIndex(midPrice, level.Price)

		if index < 0 {
			continue
		}

		grid.density[index] += level.Qty
	}

	for _, level := range asks {
		index := grid.priceIndex(midPrice, level.Price)

		if index < 0 {
			continue
		}

		grid.density[index] += level.Qty
	}
}

func (grid *FluidGrid) priceIndex(midPrice, price float64) int {
	offset := int(math.Round((price - midPrice) / grid.tickSize))
	index := grid.midIndex + offset

	if index < 0 || index >= len(grid.density) {
		return -1
	}

	return index
}

func (grid *FluidGrid) computeSourceSink(elapsed float64) {
	for index := range grid.density {
		grid.sourceSink[index] = (grid.density[index] - grid.previousDensity[index]) / elapsed
	}
}

func (grid *FluidGrid) computeVelocity(elapsed float64, midPrice float64) {
	grid.clearField(grid.velocity)

	if grid.prevMidPrice > 0 {
		grid.midPriceVelocity = (midPrice - grid.prevMidPrice) / elapsed
	}

	for index := range grid.velocity {
		grid.velocity[index] = grid.midPriceVelocity
	}

	grid.prevMidPrice = midPrice
}

func (grid *FluidGrid) replenishment(elapsed float64) float64 {
	positive := 0.0
	touchBand := int(float64(grid.halfWidth) * touchBandFraction)

	if touchBand < 1 {
		touchBand = 1
	}

	for index, rate := range grid.sourceSink {
		if rate <= 0 {
			continue
		}

		if absInt(index-grid.midIndex) > touchBand {
			continue
		}

		positive += rate
	}

	if positive <= 0 {
		return 0
	}

	return positive * elapsed
}

func (grid *FluidGrid) midMomentumDivergence() float64 {
	index := grid.midIndex

	if index <= 0 || index >= len(grid.density)-1 {
		return 0
	}

	fluxRight := grid.density[index+1] * grid.velocity[index+1]
	fluxLeft := grid.density[index-1] * grid.velocity[index-1]

	return (fluxRight - fluxLeft) / (2 * grid.tickSize)
}

func (grid *FluidGrid) viscosity() float64 {
	return grid.replenishmentRate
}

func (grid *FluidGrid) reynolds(spread float64) float64 {
	if spread <= 0 {
		return math.NaN()
	}

	viscosity := grid.replenishmentRate

	if viscosity <= 0 {
		return math.Inf(1)
	}

	return math.Abs(grid.midPriceVelocity) * spread / viscosity
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}

	return value
}
