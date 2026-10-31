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
Focus returns the dashboard-selected symbol for wire publishes.
*/
func Focus() string {
	symbol, _ := uiFocus.Load().(string)
	return symbol
}

/*
Allows reports whether symbol may appear on a focus-gated wire frame.
Empty focus suppresses every symbol.
*/
func Allows(symbol string) bool {
	focus := Focus()
	if focus == "" || symbol == "" {
		return false
	}

	return symbol == focus
}
