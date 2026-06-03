package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

func newTestCrypto() *Crypto {
	return &Crypto{
		streams:   focus.NewSet(),
		inventory: map[string]float64{},
		avgEntry:  map[string]float64{},
		pending:   map[string]struct{}{},
	}
}

func buyExec(symbol string, qty, price float64) map[string]any {
	return map[string]any{
		"channel": "executions", "symbol": symbol,
		"side": string(trading.Buy), "qty": qty, "price": price,
	}
}

func sellExec(symbol string, qty float64) map[string]any {
	return map[string]any{
		"channel": "executions", "symbol": symbol,
		"side": string(trading.Sell), "qty": qty, "price": 1.0,
	}
}

func TestObserveExecution(t *testing.T) {
	Convey("Given a flat trader", t, func() {
		crypto := newTestCrypto()
		crypto.pending["BTC/EUR"] = struct{}{}

		Convey("A buy fill opens the position and marks it held", func() {
			crypto.observeExecution(buyExec("BTC/EUR", 0.5, 100))

			So(crypto.inventory["BTC/EUR"], ShouldEqual, 0.5)
			So(crypto.avgEntry["BTC/EUR"], ShouldEqual, 100)
			So(crypto.streams.Has("BTC/EUR"), ShouldBeTrue)
			_, stillPending := crypto.pending["BTC/EUR"]
			So(stillPending, ShouldBeFalse)
		})

		Convey("A settling sell closes the position and clears holding", func() {
			crypto.observeExecution(buyExec("BTC/EUR", 0.5, 100))
			crypto.observeExecution(sellExec("BTC/EUR", 0.5))

			_, held := crypto.inventory["BTC/EUR"]
			So(held, ShouldBeFalse)
			So(crypto.streams.Has("BTC/EUR"), ShouldBeFalse)
		})

		Convey("A zero-quantity execution only clears the in-flight marker", func() {
			crypto.observeExecution(map[string]any{
				"channel": "executions", "symbol": "BTC/EUR",
				"side": string(trading.Buy), "qty": 0.0, "price": 0.0,
			})

			_, held := crypto.inventory["BTC/EUR"]
			So(held, ShouldBeFalse)
			_, stillPending := crypto.pending["BTC/EUR"]
			So(stillPending, ShouldBeFalse)
		})
	})
}

func TestSubmitGate(t *testing.T) {
	Convey("Given the not-holding gate", t, func() {
		crypto := newTestCrypto()
		entry := perspectives.Action{
			Type: perspectives.ActionLimit, Symbol: "BTC/EUR",
			Side: trading.Buy, Quantity: 1, Price: 100,
		}

		Convey("An entry is skipped while the symbol is already held", func() {
			crypto.inventory["BTC/EUR"] = 1
			crypto.streams.Add("BTC/EUR")

			// desk is nil: reaching it would panic, so a clean return proves the gate.
			crypto.submit(entry)

			So(crypto.streams.Snapshot(), ShouldResemble, []string{"BTC/EUR"})
		})

		Convey("An entry is skipped while an order is in flight", func() {
			crypto.pending["BTC/EUR"] = struct{}{}

			crypto.submit(entry)

			So(crypto.inventory["BTC/EUR"], ShouldEqual, 0)
		})

		Convey("An exit is skipped when nothing is held", func() {
			exit := perspectives.Action{
				Type: perspectives.ActionSettlePosition, Symbol: "BTC/EUR",
			}

			crypto.submit(exit)

			_, stillPending := crypto.pending["BTC/EUR"]
			So(stillPending, ShouldBeFalse)
		})
	})
}
