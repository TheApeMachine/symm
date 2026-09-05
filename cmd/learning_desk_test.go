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

	})
}

func TestRecordLifecycle(t *testing.T) {
	Convey("Given distinct entry, reduction and exit instructions on one lot", t, func() {
		bridge := stalledBridge(1)
		meter := strategy.NewRealizationMeter()
		bridge.AttachRealization(meter)
		for _, action := range []struct {
			id     string
			price  int64
			reduce bool
		}{
			{"entry", 100, false}, {"reduce", 120, true}, {"exit", 80, true},
		} {
			intent := entryIntent("TEST/USD")
			intent.CorrelationID, intent.Reference, intent.Reduce = action.id, big.NewRat(action.price, 1), action.reduce
			bridge.inFlight.Store(action.id, intent)
		}
		event := hindsight.LifecycleEvent{
			DecisionID: "entry", ActionCorrelationID: "exit", Symbol: "TEST/USD",
			Kind: "execution_terminal", Execution: &hindsight.ExecutionFact{AvgPrice: "80", CumQty: "1", OrderStatus: "filled"},
		}
		Convey("an exit uses its own reference even while the entry mapping exists", func() {
			bridge.RecordLifecycle(event)
			So(meter.AllowsTrading(), ShouldBeTrue)
			_, found := bridge.inFlight.Load("exit")
			So(found, ShouldBeFalse)
			_, found = bridge.inFlight.Load("entry")
			So(found, ShouldBeTrue)
			event.Execution.AvgPrice = "1"
			bridge.RecordLifecycle(event)
			So(meter.AllowsTrading(), ShouldBeTrue)
			event.ActionCorrelationID = "reduce"
			event.Execution.AvgPrice = "120"
			bridge.RecordLifecycle(event)
			So(meter.AllowsTrading(), ShouldBeTrue)
			_, found = bridge.inFlight.Load("reduce")
			So(found, ShouldBeFalse)
		})
		Convey("an uncorrelated exit never uses the entry decision or symbol", func() {
			event.ActionCorrelationID = ""
			event.Execution.AvgPrice = "1"
			bridge.inFlight.Store(event.Symbol, entryIntent(event.Symbol))
			bridge.RecordLifecycle(event)
			So(meter.AllowsTrading(), ShouldBeTrue)
		})
		Convey("a genuinely adverse exit still trips realization", func() {
			event.Execution.AvgPrice = "78"
			bridge.RecordLifecycle(event)
			So(meter.AllowsTrading(), ShouldBeFalse)
		})
		Convey("cancellation without fills releases the mapping without inventing slippage", func() {
			event.Execution = &hindsight.ExecutionFact{OrderStatus: "canceled"}
			bridge.RecordLifecycle(event)
			_, found := bridge.inFlight.Load("exit")
			So(found, ShouldBeFalse)
			So(meter.AllowsTrading(), ShouldBeTrue)
		})
		Convey("cleanup also runs without an attached meter", func() {
			bridge.realization = nil
			bridge.RecordLifecycle(event)
			_, found := bridge.inFlight.Load("exit")
			So(found, ShouldBeFalse)
		})
	})
}

func BenchmarkRecordLifecycle(b *testing.B) {
	bridge := stalledBridge(1)
	bridge.AttachRealization(strategy.NewRealizationMeter())
	intent := entryIntent("TEST/USD")
	event := hindsight.LifecycleEvent{ActionCorrelationID: "action", Kind: "execution_terminal",
		Execution: &hindsight.ExecutionFact{AvgPrice: "2", CumQty: "1", OrderStatus: "filled"}}
	b.ReportAllocs()
	for b.Loop() {
		bridge.inFlight.Store("action", intent)
		bridge.RecordLifecycle(event)
	}
}
