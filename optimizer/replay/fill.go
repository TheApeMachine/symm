package replay

import (
	"fmt"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
executionFill is the replay ledger's taker-style fill quote for one quantity.
*/
type executionFill struct {
	price         float64
	slippagePct   float64
	depthCoverage float64
}

/*
takerFill delegates to broker.SlippageFill / StressedSlippageReplayFill so optimizer
scoring uses the same book walk as the desk and paper matcher.
*/
func takerFill(
	costs ReplayCosts,
	measurement types.Measurement,
	side trading.Side,
	quantity float64,
	snapshots []types.Measurement,
) (executionFill, error) {
	if quantity <= 0 {
		return executionFill{}, fmt.Errorf("replay fill: quantity must be positive")
	}

	quote := broker.QuoteFromMeasurement(measurement)

	if quote.Last <= 0 && quote.Bid <= 0 && quote.Ask <= 0 {
		return executionFill{}, fmt.Errorf(
			"replay fill: missing reference price for %s",
			measurement.Symbol,
		)
	}

	var fill broker.FillQuote
	var err error

	if costs.ExecutionStressEnabled {
		fill, err = broker.StressedSlippageReplayFill(quote, side, quantity, snapshots)
	} else {
		fill, err = broker.SlippageFill(quote, side, quantity)
	}

	if err != nil {
		return executionFill{}, err
	}

	return executionFill{
		price:         fill.Price,
		slippagePct:   fill.SlippageBps / 10_000,
		depthCoverage: fill.DepthCoverage,
	}, nil
}
