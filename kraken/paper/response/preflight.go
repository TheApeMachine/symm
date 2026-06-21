package response

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

func (fillSimulator *FillSimulator) preflightGatesAt(
	order *datura.Artifact,
	quote *datura.Artifact,
	now time.Time,
) error {
	if order == nil || quote == nil {
		return fmt.Errorf("paper preflight: order or quote artifact is nil")
	}

	quantity := datura.Peek[float64](order, "order_qty")

	if quantity <= 0 {
		return fmt.Errorf("paper preflight: quantity must be positive")
	}

	actionType := logic.ActionType(datura.Peek[string](order, "action_type"))

	if actionType.IsExit() {
		if !fillSimulator.usableExitReference(quote) {
			scope, _ := quote.Scope()

			return fmt.Errorf("paper preflight: incomplete quote for exit %s", scope)
		}

		if err := fillSimulator.preflightQuoteFreshness(quote, now); err != nil {
			return fmt.Errorf("paper preflight: stale last price for exit: %w", err)
		}

		return nil
	}

	if datura.Peek[float64](quote, "bid") <= 0 || datura.Peek[float64](quote, "ask") <= 0 {
		scope, _ := quote.Scope()

		return fmt.Errorf("paper preflight: incomplete quote for %s", scope)
	}

	if err := fillSimulator.preflightQuoteFreshness(quote, now); err != nil {
		return err
	}

	if err := fillSimulator.preflightSpread(quote); err != nil {
		return err
	}

	if datura.Peek[string](order, "order_type") == "limit" {
		return nil
	}

	return fillSimulator.preflightMarketSlippage(order, quote)
}

func (fillSimulator *FillSimulator) usableExitReference(quote *datura.Artifact) bool {
	if datura.Peek[float64](quote, "last") > 0 {
		return true
	}

	return datura.Peek[float64](quote, "bid") > 0 &&
		datura.Peek[float64](quote, "ask") > 0
}

func (fillSimulator *FillSimulator) preflightQuoteFreshness(
	quote *datura.Artifact,
	now time.Time,
) error {
	maxAge := viper.GetDuration("trading.max_quote_age")

	if maxAge <= 0 {
		return nil
	}

	raw := datura.Peek[string](quote, "updated_at")

	if raw == "" {
		scope, _ := quote.Scope()

		return fmt.Errorf("paper preflight: missing quote timestamp for %s", scope)
	}

	updatedAt, err := time.Parse(time.RFC3339Nano, raw)

	if err != nil {
		scope, _ := quote.Scope()

		return fmt.Errorf("paper preflight: missing quote timestamp for %s", scope)
	}

	if now.Sub(updatedAt) > maxAge {
		scope, _ := quote.Scope()

		return fmt.Errorf("paper preflight: stale quote for %s", scope)
	}

	return nil
}

func (fillSimulator *FillSimulator) preflightSpread(quote *datura.Artifact) error {
	maxSpreadBps := viper.GetFloat64("trading.max_spread_bps")

	if maxSpreadBps <= 0 {
		return nil
	}

	spreadBps := fillSimulator.midSpreadBps(quote) * 2

	if spreadBps > maxSpreadBps {
		scope, _ := quote.Scope()

		return fmt.Errorf(
			"paper preflight: spread %.2f bps exceeds limit %.2f for %s",
			spreadBps,
			maxSpreadBps,
			scope,
		)
	}

	return nil
}

func (fillSimulator *FillSimulator) preflightMarketSlippage(
	order *datura.Artifact,
	quote *datura.Artifact,
) error {
	side := datura.Peek[string](order, "side")
	quantity := datura.Peek[float64](order, "order_qty")

	fill, err := fillSimulator.slippageFill(quote, side, quantity)

	if err != nil {
		return err
	}

	defer fill.Release()

	maxSlippageBps := viper.GetFloat64("trading.max_slippage_bps")

	if maxSlippageBps <= 0 {
		maxSlippageBps = viper.GetFloat64("trading.paper.slippage_bps") * 2
	}

	fillSlippageBps := datura.Peek[float64](fill, "slippage_bps")

	if maxSlippageBps > 0 && fillSlippageBps > maxSlippageBps {
		scope, _ := quote.Scope()

		return fmt.Errorf(
			"paper preflight: projected slippage %.2f bps exceeds limit %.2f for %s",
			fillSlippageBps,
			maxSlippageBps,
			scope,
		)
	}

	minCoverage := viper.GetFloat64("trading.replay.min_depth_coverage")

	if minCoverage <= 0 {
		minCoverage = 1
	}

	levels := fillSimulator.depthLevels(quote, side)

	if len(levels) == 0 {
		return nil
	}

	depthCoverage := datura.Peek[float64](fill, "depth_coverage")

	if depthCoverage < minCoverage {
		scope, _ := quote.Scope()

		return fmt.Errorf(
			"paper preflight: insufficient book depth for %s (coverage %.2f)",
			scope,
			depthCoverage,
		)
	}

	return nil
}
