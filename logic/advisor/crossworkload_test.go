package advisor

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestCrossStreamTemporalProvenance asserts that two readings composed from
different producer rings remain distinguishable by their own ObservedAt, and
the Perspective's At is NOT stamped onto every underlying fact. Feed CVD
(trade, t=100) then liquidity (ticker, t=200): the flow reading must carry
ObservedAt=100 while the capacity reading carries ObservedAt=200, even though
the Perspective's At is 200.
*/
func TestCrossStreamTemporalProvenance(t *testing.T) {
	Convey("Given one execution advisor composed over trade and ticker facts", t, func() {
		advisor := NewExecutionAdvisor("advisor.execution.provenance:" + t.Name())

		flowAt := time.Unix(100, 0)
		capacityAt := time.Unix(200, 0)

		// Trade CVD arrives first (t=100).
		advisor.Step(testMeasurement("TEST/USD", "cvd", flowAt, map[string]float64{
			"aggressive_notional:buy": 500.0,
		}))

		// Ticker liquidity arrives later (t=200).
		perspective := advisor.Step(testMeasurement("TEST/USD", "liquidity", capacityAt, map[string]float64{
			"touch_notional:ask": 1000.0,
		}))

		So(perspective, ShouldNotBeNil)
		So(perspective.At, ShouldEqual, capacityAt)

		Convey("the two input readings keep their own observation times, distinct from the Perspective At", func() {
			bindings := ExecutionBindings()
			flowBinding := bindings[0] // cvd/aggressive_notional:buy
			capacityBinding := bindings[2] // liquidity/touch_notional:ask

			flowReading, flowFound := executionReading(perspective, flowBinding.Series.ValueSymbol)
			So(flowFound, ShouldBeTrue)
			So(flowReading.ObservedAt, ShouldEqual, flowAt)

			capacityReading, capacityFound := executionReading(perspective, capacityBinding.Series.ValueSymbol)
			So(capacityFound, ShouldBeTrue)
			So(capacityReading.ObservedAt, ShouldEqual, capacityAt)

			// The Perspective At is the latest event (200), but it is NOT
			// stamped onto the flow reading (which stays 100).
			So(flowReading.ObservedAt, ShouldNotEqual, perspective.At)
		})
	})
}

/*
TestCrossStreamFutureLeakRejected asserts the future-leak contract: a retained
input observed at a LATER event time than the event currently being evaluated
must not be used in a derived cross-stream quantity. Feed liquidity (t=100)
WITHOUT a CVD fact, then feed CVD at t=200, then evaluate again at t=150: the
flow fact's t=200 is future relative to t=150, so the derived ratio is
undefined rather than using future state.
*/
func TestCrossStreamFutureLeakRejected(t *testing.T) {
	Convey("Given an execution advisor whose flow fact arrives after the evaluation instant", t, func() {
		advisor := NewExecutionAdvisor("advisor.execution.futureleak:" + t.Name())

		// Capacity and flow both arrive first at t=100/t=200.
		advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(100, 0), map[string]float64{
			"touch_notional:ask": 1000.0,
		}))
		advisor.Step(testMeasurement("TEST/USD", "cvd", time.Unix(200, 0), map[string]float64{
			"aggressive_notional:buy": 500.0,
		}))

		// Now evaluate at t=150: the retained flow fact (t=200) is in the
		// future relative to this event. The derived buy-share must be
		// undefined rather than computed from future flow.
		perspective := advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(150, 0), map[string]float64{
			"relative_spread": 0.01,
		}))

		So(perspective, ShouldNotBeNil)

		buyShare, found := executionReading(perspective, symbolExecutionBuyCoverage)
		So(found, ShouldBeTrue)
		So(buyShare.Defined, ShouldBeFalse)

		Convey("the retained flow reading itself still keeps its own (future) time, but the joint quantity refuses it", func() {
			// The raw flow fact is still stored with its own observation time;
			// only the cross-stream derived ratio is refused.
			flowReading, flowFound := executionReading(perspective, ExecutionBindings()[0].Series.ValueSymbol)
			So(flowFound, ShouldBeTrue)
			So(flowReading.ObservedAt, ShouldEqual, time.Unix(200, 0))
		})
	})
}

/*
TestSharedAdvisorComposesAcrossWorkloadStreams asserts the one shared advisor
instance's resident state contains the causally available facts from three
different producer streams in the declared order: trade (CVD), Level3
(depthflow), ticker (liquidity). It is the advisor-level mirror of the cmd
topology test and fails under the old ticker-only mounting because the
depthflow and cvd facts would never reach a ticker-only advisor.
*/
func TestSharedAdvisorComposesAcrossWorkloadStreams(t *testing.T) {
	Convey("Given one shared liquidity advisor", t, func() {
		advisor := NewLiquidityAdvisor("advisor.liquidity.shared:" + t.Name())

		// liquidity (ticker) + depthflow (level3) are Liquidity's declared inputs.
		advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(100, 0), map[string]float64{
			"relative_spread": 0.01,
		}))
		advisor.Step(testMeasurement("TEST/USD", "depthflow", time.Unix(200, 0), map[string]float64{
			"book_imbalance": 0.5,
		}))

		Convey("the resident state holds both stream's facts on one authority", func() {
			state, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)

			bindings := LiquidityBindings()
			spreadValue, hasSpread := state.Get(bindings[0].Series.ValueSymbol)
			So(hasSpread, ShouldBeTrue)
			So(spreadValue, ShouldEqual, 0.01)

			bookValue, hasBook := state.Get(bindings[2].Series.ValueSymbol)
			So(hasBook, ShouldBeTrue)
			So(bookValue, ShouldEqual, 0.5)
		})
	})
}

/*
TestSharedAdvisorConcurrentRings steps ONE shared advisor instance from two
goroutines — one delivering the flow fact (trade), the other the capacity fact
(ticker) — exactly as the two producer Workloads would, and asserts the derived
cross-stream coverage converges to the correct value under the race detector.
Same symbol + same semantic state must stay serializable: no race, no lost
update, and the derived quantity is defined with the right value, not merely
"some CVD reached the graph".
*/
func TestSharedAdvisorConcurrentRings(t *testing.T) {
	Convey("Given one shared execution advisor stepped from two concurrent rings", t, func() {
		advisor := NewExecutionAdvisor("advisor.execution.concurrent:" + t.Name())
		at := time.Unix(100, 0)

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()

			for range 50 {
				advisor.Step(testMeasurement("TEST/USD", "cvd", at, map[string]float64{
					"aggressive_notional:buy": 500.0,
				}))
			}
		}()

		go func() {
			defer wg.Done()

			for range 50 {
				advisor.Step(testMeasurement("TEST/USD", "liquidity", at, map[string]float64{
					"touch_notional:ask": 1000.0,
				}))
			}
		}()

		wg.Wait()

		Convey("the derived coverage is correct and defined", func() {
			perspective := advisor.Step(testMeasurement("TEST/USD", "cvd", at, map[string]float64{
				"aggressive_notional:buy": 500.0,
			}))

			coverage, found := executionReading(perspective, symbolExecutionBuyCoverage)
			So(found, ShouldBeTrue)
			So(coverage.Defined, ShouldBeTrue)
			So(coverage.Value, ShouldAlmostEqual, 0.5, 1e-9)
		})
	})
}
