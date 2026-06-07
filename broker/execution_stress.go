package broker

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/execution"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
ExecutionStressMultiplier delegates proprietary stress math to execution.StressMultiplier.
*/
func ExecutionStressMultiplier(snapshots []types.Measurement) float64 {
	return execution.StressMultiplier(snapshots)
}

/*
ExecutionStressFromSymbol approximates replay stress on the paper path from the desk
stress cache and the latest published market regime.
*/
func ExecutionStressFromSymbol(stress SymbolStress) float64 {
	return execution.StressFromHostileSNR(stress.hostileStress(), perspectives.CurrentRegime())
}

/*
RegimeHostility delegates to execution.RegimeHostility.
*/
func RegimeHostility(regime types.Regime) float64 {
	return execution.RegimeHostility(regime)
}

/*
StressedSlippageReplayFill applies replay stress from a measurement window snapshot.
*/
func StressedSlippageReplayFill(
	quote Quote,
	side trading.Side,
	qty float64,
	snapshots []types.Measurement,
) (FillQuote, error) {
	fill, err := SlippageFill(quote, side, qty)

	if err != nil {
		return FillQuote{}, err
	}

	if !viper.GetBool("trading.replay.execution_stress_enabled") {
		return fill, nil
	}

	stressed, err := applyExecutionStress(fill, quote, side, execution.StressMultiplier(snapshots))

	return stressed, err
}

func applyExecutionStress(
	fill FillQuote,
	quote Quote,
	side trading.Side,
	multiplier float64,
) (FillQuote, error) {
	// Anchor stress at the achieved book-walk price (which already includes the
	// spread and depth cost), never at last/mid: a "stressed" fill must not be
	// better than the unstressed one. Re-deriving from quote.Last let buys fill
	// at or below the bid whenever the last print sat on the other side.
	anchor := fill.Price

	if anchor <= 0 {
		anchor = sideTouchPrice(quote, side)
	}

	hasDepth := len(quote.Book.Bids) > 0 || len(quote.Book.Asks) > 0
	depthCoverage := fill.DepthCoverage

	if !hasDepth {
		depthCoverage = 1
	}

	price, slippageBps, err := execution.StressedFillQuote(
		side,
		anchor,
		fill.SlippageBps,
		depthCoverage,
		multiplier,
		defaultPaperSlippagePct(),
	)

	if err != nil {
		return FillQuote{}, err
	}

	priceCovered := fill.PriceCovered

	if priceCovered > 0 && fill.Price > 0 {
		priceCovered *= price / fill.Price // carry the stress ratio onto the covered-only price
	}

	return FillQuote{
		Price:         price,
		PriceCovered:  priceCovered,
		SlippageBps:   slippageBps,
		DepthCoverage: fill.DepthCoverage,
	}, nil
}

/*
sideTouchPrice is the taker's entry point on a quote: the ask for a buy, the bid
for a sell, falling back to last then mid only when the touch is unknown.
*/
func sideTouchPrice(quote Quote, side trading.Side) float64 {
	if side == trading.Buy && quote.Ask > 0 {
		return quote.Ask
	}

	if side == trading.Sell && quote.Bid > 0 {
		return quote.Bid
	}

	if quote.Last > 0 {
		return quote.Last
	}

	if quote.Bid > 0 && quote.Ask > 0 {
		return (quote.Bid + quote.Ask) / 2
	}

	return 0
}

/*
StressedSlippageFill walks the book and inflates slippage for hostile flow, regime,
and depth shortfall — matching the replay ledger's taker model.
*/
func StressedSlippageFill(
	quote Quote,
	side trading.Side,
	qty float64,
	stress SymbolStress,
) (FillQuote, error) {
	fill, err := SlippageFill(quote, side, qty)

	if err != nil {
		return FillQuote{}, err
	}

	if !viper.GetBool("trading.replay.execution_stress_enabled") {
		return fill, nil
	}

	return applyExecutionStress(fill, quote, side, ExecutionStressFromSymbol(stress))
}

func defaultPaperSlippagePct() float64 {
	slippageBps := viper.GetFloat64("trading.paper.slippage_bps")

	if slippageBps > 0 {
		return slippageBps / 10_000
	}

	return 0
}
