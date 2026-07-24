package types

import (
	"sync/atomic"
)

/*
uiFocus holds the dashboard-selected symbol so focus-gated wire publishes can
drop non-focus signal metrics without changing the trading thesis path.
*/
var uiFocus atomic.Value

func init() {
	uiFocus.Store("")
}

/*
SetFocus records the symbol the dashboard wants signal metrics for. Empty
clears the gate so publishers may emit the full cross-section again.
*/
func SetFocus(symbol string) {
	uiFocus.Store(symbol)
}

/*
Focus returns the current dashboard focus symbol, or empty when ungated.
*/
func Focus() string {
	value := uiFocus.Load()

	if value == nil {
		return ""
	}

	symbol, _ := value.(string)

	return symbol
}

/*
Focused returns rows unchanged when focus is empty, otherwise only rows whose
Symbol matches the dashboard focus. Thesis Publish stays ungated; only the UI
wire path uses this filter.
*/
func Focused(rows []*Measurement) []*Measurement {
	symbol := Focus()

	if symbol == "" || len(rows) == 0 {
		return rows
	}

	out := make([]*Measurement, 0, len(rows))

	for _, row := range rows {
		if row == nil || row.Symbol != symbol {
			continue
		}

		out = append(out, row)
	}

	return out
}
