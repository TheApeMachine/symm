package cmd

import (
	"math/big"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
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

		Convey("When realization is attached, dropped submissions report to realization", func() {
			meter := strategy.NewRealizationMeter()
			bridge.AttachRealization(meter)

			So(meter.AllowsTrading(), ShouldBeTrue)
			// Queue capacity is 2; sending 2 fills the queue, next 3 drop (3 consecutive failures)
			for range 5 {
				_ = bridge.Submit(entryIntent("TEST/USD"))
			}

			So(meter.AllowsTrading(), ShouldBeFalse)
			So(meter.Reason(), ShouldContainSubstring, "consecutive execution submission failures exceeded threshold")
		})

		Convey("Authoritative fill records report realized slippage to RealizationMeter", func() {
			meter := strategy.NewRealizationMeter()
			bridge.AttachRealization(meter)

			intent := entryIntent("TEST/USD")
			intent.CorrelationID = "decision-123"
			intent.Reference = big.NewRat(100, 1) // ref price = 100.0
			bridge.inFlight.Store("decision-123", intent)

			// Clean fill at 100.1 (10 bps slippage)
			bridge.RecordLifecycle(hindsight.LifecycleEvent{
				DecisionID: "decision-123",
				Symbol:     "TEST/USD",
				Kind:       "entry_fill",
				Execution: &hindsight.ExecutionFact{
					AvgPrice: "100.1",
				},
			})

			So(meter.AllowsTrading(), ShouldBeTrue)

			// Catastrophic fill at 102.0 (200 bps slippage > 150 bps bound)
			bridge.RecordLifecycle(hindsight.LifecycleEvent{
				DecisionID: "decision-123",
				Symbol:     "TEST/USD",
				Kind:       "entry_fill",
				Execution: &hindsight.ExecutionFact{
					AvgPrice: "102.0",
				},
			})

			So(meter.AllowsTrading(), ShouldBeFalse)
			So(meter.Reason(), ShouldContainSubstring, "catastrophic single-fill slippage exceeded bound")
		})
	})
}
