package broker

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market"
)

/*
PreflightGates rejects orders when quote quality or projected slippage is unacceptable.
Both paper and live desks call this before submitting an order.
*/
func PreflightGates(
	quote Quote,
	side trading.Side,
	qty float64,
	orderType trading.OrderType,
) error {
	if quote.Bid <= 0 || quote.Ask <= 0 {
		return fmt.Errorf("preflight: incomplete quote for %s", quote.Symbol)
	}

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

	if orderType == trading.Limit {
		return nil
	}

	fill, err := SlippageFill(quote, side, qty)

	if err != nil {
		return err
	}

	maxSlippageBps, err := market.RequiredFloat("trading.max_slippage_bps")

	if err != nil {
		return fmt.Errorf("preflight: %w", err)
	}

	if fill.SlippageBps > maxSlippageBps {
		return fmt.Errorf(
			"preflight: projected slippage %.2f bps exceeds limit %.2f for %s",
			fill.SlippageBps,
			maxSlippageBps,
			quote.Symbol,
		)
	}

	return nil
}
