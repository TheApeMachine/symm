package trader

import "time"

/*
ConnectSnapshot returns the events a fresh dashboard client needs on websocket connect:
wallet status, open positions, and synthetic trade_enter rows for each hold.
*/
func (crypto *Crypto) ConnectSnapshot() []map[string]any {
	events := make([]map[string]any, 0, 1+len(crypto.holds))
	events = append(events, crypto.statusEvent())
	events = append(events, crypto.openTradeEnterEvents()...)

	return events
}

func (crypto *Crypto) openTradeEnterEvents() []map[string]any {
	if len(crypto.holds) == 0 {
		return nil
	}

	events := make([]map[string]any, 0, len(crypto.holds))

	for symbol, hold := range crypto.holds {
		last, _, _, _, ok := crypto.quote(symbol)

		if !ok || last <= 0 {
			last = hold.entryPrice
		}

		enteredAt := hold.enteredAt.UTC().Format(time.RFC3339Nano)

		events = append(events, map[string]any{
			"event":        "trade_enter",
			"ts":           enteredAt,
			"symbol":       symbol,
			"regime":       hold.regime,
			"reason":       hold.reason,
			"score":        hold.confidence,
			"trail_pct":    hold.trailPct,
			"fill":         hold.entryPrice,
			"stop":         hold.stopPrice,
			"notional_eur": hold.notional,
			"last":         last,
		})
	}

	return events
}
