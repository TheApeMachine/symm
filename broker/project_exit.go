package broker

import (
	"fmt"

	"github.com/theapemachine/symm/kraken/trading"
)

/*
ProjectExitBalance reports the quote-currency cash balance after selling every
held lot at market via SlippageFill and paying takerFeePct on each leg. Lots with
qty <= 0 are skipped. Returns an error when any held lot cannot be priced.
*/
func ProjectExitBalance(
	cash float64,
	inventory map[string]float64,
	quotes *QuoteCache,
	takerFeePct float64,
) (float64, error) {
	if quotes == nil {
		return 0, fmt.Errorf("project exit balance: quote cache required")
	}

	if takerFeePct < 0 {
		return 0, fmt.Errorf("project exit balance: taker fee pct must be non-negative")
	}

	feeRate := takerFeePct / 100
	total := cash

	for symbol, qty := range inventory {
		if qty <= 0 {
			continue
		}

		if symbol == "" {
			return 0, fmt.Errorf("project exit balance: empty symbol")
		}

		quote, ok := quotes.Snapshot(symbol)

		if !ok {
			return 0, fmt.Errorf("project exit balance: no quote for %s", symbol)
		}

		fill, err := SlippageFill(quote, trading.Sell, qty)

		if err != nil {
			return 0, fmt.Errorf("project exit balance: %s: %w", symbol, err)
		}

		proceeds := qty * fill.Price
		total += proceeds * (1 - feeRate)
	}

	return total, nil
}
