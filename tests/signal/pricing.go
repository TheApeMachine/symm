package signal

import (
	"fmt"
	"math"
	"time"

	testtypes "github.com/theapemachine/symm/tests/types"
)

/*
NewGeneratorFromSymbol preserves all per-instrument precision, spread, depth,
and cross-market characteristics declared by a scenario symbol profile.
*/
func NewGeneratorFromSymbol(symbol *testtypes.Symbol) *Generator {
	generator := NewGenerator(
		symbol.Pair,
		symbol.StartPrice,
		symbol.PriceIncrement,
		symbol.PricePrecision,
		symbol.Seed,
	)

	generator.quantityScale = math.Pow10(symbol.QuantityPrecision)
	generator.spreadFraction = symbol.BaseSpreadFraction
	generator.factorLoading = symbol.FactorLoading
	generator.depthLevels = symbol.BookDepthLevels
	generator.depthScale = symbol.DepthQuantityScale

	return generator
}

/*
ConfigureProfiles installs a scenario-owned regime contract.
*/
func (generator *Generator) ConfigureProfiles(
	profiles map[testtypes.MarketState]testtypes.RegimeProfile,
) error {
	baseline, known := profiles[testtypes.Baseline]

	if !known {
		return fmt.Errorf("generator: baseline regime profile is required")
	}

	generator.mu.Lock()
	generator.profiles = testtypes.CloneProfiles(profiles)
	generator.sourceProfile = baseline
	generator.mu.Unlock()

	return nil
}

func (generator *Generator) transitionMomentum(
	state testtypes.MarketState,
	momentum []float64,
) float64 {
	if _, known := generator.profiles[state]; !known {
		panic("generator: unknown market state")
	}

	if len(momentum) > 1 {
		panic("generator: state accepts at most one momentum value")
	}

	if len(momentum) == 0 || momentum[0] == 0 {
		return 1
	}

	return math.Abs(momentum[0])
}

/*
SetTime establishes the next scenario observation from an explicit replay
anchor instead of the host wall clock.
*/
func (generator *Generator) SetTime(start time.Time) error {
	if start.IsZero() {
		return fmt.Errorf("generator: replay start time is required")
	}

	generator.mu.Lock()
	generator.currTime = start.UTC()
	generator.mu.Unlock()

	return nil
}

/*
ConfigureDepth sets the finite generated tiers consumed by book rendering and
the execution venue.
*/
func (generator *Generator) ConfigureDepth(levels int, quantityScale float64) error {
	if levels < 1 || quantityScale <= 0 || math.IsNaN(quantityScale) ||
		math.IsInf(quantityScale, 0) {
		return fmt.Errorf("generator: depth levels and quantity scale must be positive")
	}

	generator.mu.Lock()
	generator.depthLevels = levels
	generator.depthScale = quantityScale
	generator.mu.Unlock()

	return nil
}

func (generator *Generator) depth(
	bid float64,
	bidQuantity float64,
	ask float64,
	askQuantity float64,
) ([]testtypes.DepthLevel, []testtypes.DepthLevel) {
	bids := make([]testtypes.DepthLevel, 0, generator.depthLevels)
	asks := make([]testtypes.DepthLevel, 0, generator.depthLevels)

	for level := range generator.depthLevels {
		bidPrice := generator.roundPrice(
			bid - float64(level)*generator.priceIncrement,
		)

		if bidPrice <= 0 {
			break
		}

		quantityScale := math.Pow(generator.depthScale, float64(level))
		bids = append(bids, testtypes.DepthLevel{
			Price:    bidPrice,
			Quantity: generator.roundQuantity(bidQuantity * quantityScale),
		})
		asks = append(asks, testtypes.DepthLevel{
			Price: generator.roundPrice(
				ask + float64(level)*generator.priceIncrement,
			),
			Quantity: generator.roundQuantity(askQuantity * quantityScale),
		})
	}

	return bids, asks
}

func (generator *Generator) roundQuantity(quantity float64) float64 {
	return math.Round(quantity*generator.quantityScale) / generator.quantityScale
}

func (generator *Generator) floorPrice(price float64) float64 {
	return generator.roundPrice(
		math.Floor(price/generator.priceIncrement) * generator.priceIncrement,
	)
}

func (generator *Generator) ceilPrice(price float64) float64 {
	return generator.roundPrice(
		math.Ceil(price/generator.priceIncrement) * generator.priceIncrement,
	)
}

func (generator *Generator) roundPrice(price float64) float64 {
	scale := math.Pow10(generator.pricePrecision)

	return math.Round(price*scale) / scale
}
