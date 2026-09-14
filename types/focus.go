package types

import (
	"strings"
	"sync/atomic"
)

/*
uiFocus holds the dashboard-selected symbol so focus-gated wire publishes can
drop non-focus signal metrics without changing the trading thesis path.
*/
var uiFocus atomic.Value

func init() {
	uiFocus.Store("BTC/USD")
}

/*
SetFocus records the only symbol whose signal metrics may be published to the
dashboard. Empty suppresses all signal metric publication.
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

	symbol, ok := value.(string)

	if !ok {
		return ""
	}

	return symbol
}

/*
Allows reports whether the given symbol is permitted under the current focus gate.
When focus is empty, "*", or "all", all symbols are permitted.
An empty symbol (global or system metrics) is always permitted.
*/
func Allows(symbol string) bool {
	focus := Focus()

	if focus == "" || focus == "*" || focus == "all" {
		return true
	}

	if symbol == "" {
		return true
	}

	return strings.EqualFold(symbol, focus)
}

