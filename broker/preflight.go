package broker

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

/*
PreflightRequest carries quote, order, intent, and live stress for gate evaluation.
*/
type PreflightRequest struct {
	Quote      Quote
	Side       trading.Side
	Quantity   float64
	OrderType  trading.OrderType
	ActionType reasoning.ActionType
	Stress     SymbolStress
}

/*
PreflightGates rejects orders when quote quality or projected slippage is unacceptable.
Exit actions bypass volatility and stress gates so liquidations are never blocked.
*/
func PreflightGates(request PreflightRequest) error {
	if request.Quantity <= 0 {
		return fmt.Errorf("preflight: quantity must be positive")
	}

	if request.Quote.Bid <= 0 || request.Quote.Ask <= 0 {
		return fmt.Errorf("preflight: incomplete quote for %s", request.Quote.Symbol)
	}

	if reasoning.IsExitAction(request.ActionType) {
		return nil
	}

	if err := preflightQuoteQuality(request.Quote); err != nil {
		return err
	}

	if request.OrderType == trading.Limit {
		return nil
	}

	return preflightMarketSlippage(request)
}

func preflightQuoteQuality(quote Quote) error {
	maxAge, err := market.RequiredDuration("trading.max_quote_age")

	if err != nil {
		return fmt.Errorf("preflight: %w", err)
	}

	if quote.UpdatedAt.IsZero() {
		return fmt.Errorf("preflight: missing quote timestamp for %s", quote.Symbol)
	}

	if time.Since(quote.UpdatedAt) > maxAge {
		return fmt.Errorf("preflight: stale quote for %s", quote.Symbol)
	}

	maxSpreadBps, err := market.RequiredFloat("trading.max_spread_bps")

	if err != nil {
		return fmt.Errorf("preflight: %w", err)
	}

	spreadBps := MidSpreadBps(quote) * 2

	if spreadBps > maxSpreadBps {
		return fmt.Errorf(
			"preflight: spread %.2f bps exceeds limit %.2f for %s",
			spreadBps,
			maxSpreadBps,
			quote.Symbol,
		)
	}

	return nil
}

func preflightMarketSlippage(request PreflightRequest) error {
	fill, err := SlippageFill(request.Quote, request.Side, request.Quantity)

	if err != nil {
		return err
	}

	maxSlippageBps, err := market.RequiredFloat("trading.max_slippage_bps")

	if err != nil {
		return fmt.Errorf("preflight: %w", err)
	}

	maxSlippageBps = request.Stress.EntrySlippageCapBps(maxSlippageBps)

	if fill.SlippageBps > maxSlippageBps {
		return fmt.Errorf(
			"preflight: projected slippage %.2f bps exceeds limit %.2f for %s",
			fill.SlippageBps,
			maxSlippageBps,
			request.Quote.Symbol,
		)
	}

	if fill.DepthCoverage < 1 {
		return fmt.Errorf(
			"preflight: insufficient book depth for %s (coverage %.2f)",
			request.Quote.Symbol,
			fill.DepthCoverage,
		)
	}

	return nil
}
