package response

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
)

/*
PreflightRequest carries quote, order, and action metadata for paper gate evaluation.
*/
type PreflightRequest struct {
	Quote      broker.Quote
	Side       string
	Quantity   float64
	OrderType  string
	ActionType logic.ActionType
}

/*
PreflightGates rejects paper orders when quote quality or projected slippage is unacceptable.
Exit actions bypass slippage gates so liquidations are never blocked.
*/
func PreflightGates(request PreflightRequest) error {
	return PreflightGatesAt(request, time.Now().UTC())
}

/*
PreflightGatesAt evaluates gates at an explicit clock for deterministic tests.
*/
func PreflightGatesAt(request PreflightRequest, now time.Time) error {
	if request.Quantity <= 0 {
		return fmt.Errorf("paper preflight: quantity must be positive")
	}

	if request.ActionType.IsExit() {
		if !usableExitReference(request.Quote) {
			return fmt.Errorf("paper preflight: incomplete quote for exit %s", request.Quote.Symbol)
		}

		if err := preflightQuoteFreshnessAt(request.Quote, now); err != nil {
			return fmt.Errorf("paper preflight: stale last price for exit: %w", err)
		}

		return nil
	}

	if request.Quote.Bid <= 0 || request.Quote.Ask <= 0 {
		return fmt.Errorf("paper preflight: incomplete quote for %s", request.Quote.Symbol)
	}

	if err := preflightQuoteFreshnessAt(request.Quote, now); err != nil {
		return err
	}

	if err := preflightSpread(request.Quote); err != nil {
		return err
	}

	if request.OrderType == "limit" {
		return nil
	}

	return preflightMarketSlippage(request)
}

func preflightFromWire(wire map[string]any, quote broker.Quote) PreflightRequest {
	actionType, _ := wire["action_type"].(string)

	return PreflightRequest{
		Quote:      quote,
		Side:       stringField(wire, "side"),
		Quantity:   floatField(wire, "order_qty"),
		OrderType:  stringField(wire, "order_type"),
		ActionType: logic.ActionType(actionType),
	}
}

func usableExitReference(quote broker.Quote) bool {
	if quote.Last > 0 {
		return true
	}

	return quote.Bid > 0 && quote.Ask > 0
}

func preflightQuoteFreshnessAt(quote broker.Quote, now time.Time) error {
	maxAge := viper.GetDuration("trading.max_quote_age")

	if maxAge <= 0 {
		return nil
	}

	if quote.UpdatedAt.IsZero() {
		return fmt.Errorf("paper preflight: missing quote timestamp for %s", quote.Symbol)
	}

	if now.Sub(quote.UpdatedAt) > maxAge {
		return fmt.Errorf("paper preflight: stale quote for %s", quote.Symbol)
	}

	return nil
}

func preflightSpread(quote broker.Quote) error {
	maxSpreadBps := viper.GetFloat64("trading.max_spread_bps")

	if maxSpreadBps <= 0 {
		return nil
	}

	spreadBps := midSpreadBps(quote) * 2

	if spreadBps > maxSpreadBps {
		return fmt.Errorf(
			"paper preflight: spread %.2f bps exceeds limit %.2f for %s",
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

	maxSlippageBps := viper.GetFloat64("trading.max_slippage_bps")

	if maxSlippageBps <= 0 {
		maxSlippageBps = viper.GetFloat64("trading.paper.slippage_bps") * 2
	}

	if maxSlippageBps > 0 && fill.SlippageBps > maxSlippageBps {
		return fmt.Errorf(
			"paper preflight: projected slippage %.2f bps exceeds limit %.2f for %s",
			fill.SlippageBps,
			maxSlippageBps,
			request.Quote.Symbol,
		)
	}

	minCoverage := viper.GetFloat64("trading.replay.min_depth_coverage")

	if minCoverage <= 0 {
		minCoverage = 1
	}

	levels := request.Quote.Book.Asks

	if request.Side == "sell" {
		levels = request.Quote.Book.Bids
	}

	if len(levels) == 0 {
		return nil
	}

	if fill.DepthCoverage < minCoverage {
		return fmt.Errorf(
			"paper preflight: insufficient book depth for %s (coverage %.2f)",
			request.Quote.Symbol,
			fill.DepthCoverage,
		)
	}

	return nil
}
