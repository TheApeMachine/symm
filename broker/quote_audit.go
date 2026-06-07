package broker

import (
	"time"

	"github.com/theapemachine/symm/kraken/trading"
)

/*
QuoteAuditFields returns quote-quality facts at decision time for audit analysis.
*/
func QuoteAuditFields(
	quote Quote,
	side trading.Side,
	quantity float64,
	now time.Time,
) map[string]any {
	fields := map[string]any{
		"bid":        quote.Bid,
		"ask":        quote.Ask,
		"last":       quote.Last,
		"spread_bps": MidSpreadBps(quote) * 2,
	}

	if !quote.UpdatedAt.IsZero() && !now.IsZero() {
		fields["quote_age_ms"] = now.Sub(quote.UpdatedAt).Milliseconds()
	}

	if quote.Volatility > 0 {
		fields["realized_vol"] = quote.Volatility
	}

	if quantity <= 0 {
		return fields
	}

	fill, err := SlippageFill(quote, side, quantity)

	if err != nil {
		return fields
	}

	fields["depth_coverage"] = fill.DepthCoverage
	fields["projected_slippage_bps"] = fill.SlippageBps

	return fields
}
