package cmd

import (
	"math/big"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
stalledBridge is a bridge whose worker is deliberately not running, standing in
for a venue that has stopped answering. Submit must behave identically either
way: it is called from the workspace consumer that feeds the terminal, and any
wait taken there stops the whole pipeline.
*/
func stalledBridge(capacity int) *learningDesk {
	return &learningDesk{
		desk:       &broker.Desk{},
		instrument: &broker.Instrument{},
		intents:    make(chan strategy.ExecutionIntent, capacity),
	}
}

func entryIntent(symbol string) strategy.ExecutionIntent {
	return strategy.ExecutionIntent{
		Symbol: symbol, Kind: types.ActionEnter,
		Quantity: big.NewRat(1, 1), Reference: big.NewRat(2, 1),
		Mode: strategy.ModeTrading,
	}
}

func TestSubmitNeverWaitsOnTheVenue(t *testing.T) {
	Convey("Given an account that has stopped accepting orders", t, func() {
		bridge := stalledBridge(2)

		Convey("Submitting past the queue drops instead of blocking", func() {
			done := make(chan struct{})

			go func() {
				for range 8 {
					_ = bridge.Submit(entryIntent("TEST/USD"))
				}

				close(done)
			}()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("Submit blocked on a stalled venue")
			}

			status := bridge.Execution()
			So(status.Queued, ShouldEqual, 2)
			So(status.Dropped, ShouldEqual, 6)
			So(status.Submitted, ShouldEqual, 0)
			So(status.Failed, ShouldEqual, 0)
		})

		Convey("An exit on a symbol the account never opened is divergence, not an order", func() {
			intent := entryIntent("TEST/USD")
			intent.Kind, intent.Reduce = types.ActionExit, true
			So(bridge.Submit(intent), ShouldBeNil)

			status := bridge.Execution()
			So(status.Diverged, ShouldEqual, 1)
			So(status.Queued, ShouldEqual, 0)
		})

		Convey("A decision with nothing to trade reaches neither queue nor counter", func() {
			intent := entryIntent("TEST/USD")
			intent.Quantity = new(big.Rat)
			So(bridge.Submit(intent), ShouldBeNil)

			status := bridge.Execution()
			So(status.Queued, ShouldEqual, 0)
			So(status.Diverged, ShouldEqual, 0)
			So(status.Dropped, ShouldEqual, 0)
		})
	})
}
