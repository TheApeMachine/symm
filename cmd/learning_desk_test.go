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
	bridge := &learningDesk{
		desk:       &broker.Desk{},
		instrument: &broker.Instrument{},
		intents:    make(chan strategy.ExecutionIntent, capacity),
	}
	bridge.executionFeedback = &executionFeedback{funds: &bridge.funds}
	return bridge
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

	})
}

func TestLearningDeskSubmit(t *testing.T) {
	Convey("Every unqueued allocation reports a terminal fact to its capital owner", t, func() {
		bridge := stalledBridge(1)
		intent := entryIntent("TEST/USD")
		intent.Allocation = &strategy.AllocationReceipt{}
		Convey("A full worker queue aborts the selected allocation", func() {
			So(bridge.Submit(entryIntent("OTHER/USD")), ShouldBeNil)
			So(bridge.Submit(intent), ShouldBeNil)
			So(intent.Allocation.Result.Load().State, ShouldEqual, "aborted")
		})
		Convey("A pre-venue repricing refusal preserves Realization", func() {
			bridge.record = func(hindsight.LearningEvent) error { return nil }
			bridge.AttachRealization(strategy.NewRealizationMeter())
			So(bridge.refused(intent, &types.ExecutionRefusal{State: "repricing failed", Detail: "book moved"}), ShouldBeNil)
			So(intent.Allocation.Result.Load().State, ShouldEqual, "aborted")
			So(bridge.realization.AllowsTrading(), ShouldBeTrue)
		})
	})
}
