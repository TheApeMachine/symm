package cmd

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/strategy"
	"math/big"
	"testing"
)

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
