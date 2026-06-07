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

	return applyExecutionStress(fill, quote, side, execution.StressMultiplier(snapshots)), nil
}

func applyExecutionStress(
	fill FillQuote,
	quote Quote,
	side trading.Side,
	multiplier float64,
) FillQuote {
	reference := quote.Last

	if reference <= 0 && quote.Bid > 0 && quote.Ask > 0 {
		reference = (quote.Bid + quote.Ask) / 2
	}

	hasDepth := len(quote.Book.Bids) > 0 || len(quote.Book.Asks) > 0
	depthCoverage := fill.DepthCoverage

	if !hasDepth {
		depthCoverage = 1
	}

	price, slippageBps := execution.StressedFillQuote(
		side,
		reference,
		fill.SlippageBps,
		depthCoverage,
		multiplier,
		defaultPaperSlippagePct(),
	)

	return FillQuote{
		Price:         price,
		SlippageBps:   slippageBps,
		DepthCoverage: fill.DepthCoverage,
	}
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

	return applyExecutionStress(fill, quote, side, ExecutionStressFromSymbol(stress)), nil
}

func defaultPaperSlippagePct() float64 {
	slippageBps := viper.GetFloat64("trading.paper.slippage_bps")

	if slippageBps > 0 {
		return slippageBps / 10_000
	}

	return 0
}
