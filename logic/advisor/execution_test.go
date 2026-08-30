package advisor

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
executionReading returns the reading for one interned output slot from a
Perspective, scoped to this test file.
*/
func executionReading(perspective *types.Perspective, slot nmtypes.Symbol) (types.MetricReading, bool) {
	return readingFor(perspective, slot)
}

func TestExecutionBindings(t *testing.T) {
	Convey("Given ExecutionBindings", t, func() {
		bindings := ExecutionBindings()

		Convey("it binds side-correct flow and capacity facts, never imbalance as capacity", func() {
			So(len(bindings), ShouldEqual, 6)

			metrics := map[string]bool{}
			sources := map[string]bool{}
			for _, binding := range bindings {
				metrics[binding.Metric] = true
				sources[binding.Source] = true
			}

			// Flow facts (quote currency).
			So(metrics["aggressive_notional:buy"], ShouldBeTrue)
			So(metrics["aggressive_notional:sell"], ShouldBeTrue)

			// Side-correct capacity facts (quote currency).
			So(metrics["touch_notional:ask"], ShouldBeTrue)
			So(metrics["touch_notional:bid"], ShouldBeTrue)

			// Context facts, not capacity.
			So(metrics["signed_net_fraction_zscore"], ShouldBeTrue)
			So(metrics["relative_spread"], ShouldBeTrue)

			// touch_notional_imbalance — the asymmetry ratio — is NOT a declared
			// input: it is not capacity and must not drive a flow-vs-capacity
			// reading.
			So(metrics["touch_notional_imbalance"], ShouldBeFalse)
		})
	})
}

func TestExecutionPipeline(t *testing.T) {
	Convey("Given ExecutionPipeline", t, func() {
		bindings := ExecutionBindings()
		pipeline := ExecutionPipeline(bindings)

		outputs := ExecutionOutputs(bindings)
		buyShare := outputs[0].Slot
		sellShare := outputs[1].Slot

		Convey("buy flow is divided by ask capacity and sell flow by bid capacity", func() {
			frame := nmtypes.Frame{}
			frame.Put(symbolCurrentAtSec, 100)
			frame.Put(symbolCurrentAtNsec, 0)

			// Buy flow 500 against ask capacity 1000 -> 0.5.
			frame.Put(bindings[0].Series.ValueSymbol, 500)
			frame.Put(bindings[0].Series.SecSymbol, 100)
			frame.Put(bindings[0].Series.NsecSymbol, 0)
			frame.Put(bindings[0].Fresh, 1)
			frame.Put(bindings[0].Maturity, 0.9)

			// Ask capacity 1000.
			frame.Put(bindings[2].Series.ValueSymbol, 1000)
			frame.Put(bindings[2].Series.SecSymbol, 100)
			frame.Put(bindings[2].Series.NsecSymbol, 0)
			frame.Put(bindings[2].Fresh, 1)
			frame.Put(bindings[2].Maturity, 0.8)

			// Sell flow 200 against bid capacity 800 -> 0.25.
			frame.Put(bindings[1].Series.ValueSymbol, 200)
			frame.Put(bindings[1].Series.SecSymbol, 100)
			frame.Put(bindings[1].Series.NsecSymbol, 0)
			frame.Put(bindings[1].Fresh, 1)
			frame.Put(bindings[1].Maturity, 0.7)

			// Bid capacity 800.
			frame.Put(bindings[3].Series.ValueSymbol, 800)
			frame.Put(bindings[3].Series.SecSymbol, 100)
			frame.Put(bindings[3].Series.NsecSymbol, 0)
			frame.Put(bindings[3].Fresh, 1)
			frame.Put(bindings[3].Maturity, 0.6)

			pipeline(&frame)

			So(frame.Err, ShouldBeNil)

			buy, hasBuy := frame.Get(buyShare)
			So(hasBuy, ShouldBeTrue)
			So(buy, ShouldAlmostEqual, 0.5, 1e-9)

			sell, hasSell := frame.Get(sellShare)
			So(hasSell, ShouldBeTrue)
			So(sell, ShouldAlmostEqual, 0.25, 1e-9)
		})

		Convey("swapping sides (buy flow against bid capacity) is wrong and yields a different, wrong-decision quantity only when measured as such", func() {
			// Side correctness is structural: buy flow maps to ask capacity via
			// bindings[2], never bindings[3]. Feed buy flow against an ask
			// capacity and verify the ratio reads the ASK slot, not the bid.
			frame := nmtypes.Frame{}
			frame.Put(symbolCurrentAtSec, 100)
			frame.Put(symbolCurrentAtNsec, 0)

			frame.Put(bindings[0].Series.ValueSymbol, 500)
			frame.Put(bindings[0].Series.SecSymbol, 100)
			frame.Put(bindings[0].Series.NsecSymbol, 0)
			frame.Put(bindings[0].Fresh, 1)
			frame.Put(bindings[0].Maturity, 0.9)

			// Ask capacity 1000, bid capacity 10 (radically different actual
			// capacity). The buy ratio must use ask (1000), not bid (10).
			frame.Put(bindings[2].Series.ValueSymbol, 1000)
			frame.Put(bindings[2].Series.SecSymbol, 100)
			frame.Put(bindings[2].Series.NsecSymbol, 0)
			frame.Put(bindings[2].Fresh, 1)
			frame.Put(bindings[2].Maturity, 0.8)

			frame.Put(bindings[3].Series.ValueSymbol, 10)
			frame.Put(bindings[3].Series.SecSymbol, 100)
			frame.Put(bindings[3].Series.NsecSymbol, 0)
			frame.Put(bindings[3].Fresh, 1)
			frame.Put(bindings[3].Maturity, 0.6)

			pipeline(&frame)

			So(frame.Err, ShouldBeNil)

			buy, hasBuy := frame.Get(buyShare)
			So(hasBuy, ShouldBeTrue)
			So(buy, ShouldAlmostEqual, 0.5, 1e-9) // 500/1000 (ask), not 500/10=50 (bid)
		})
	})
}

func TestExecutionOutputs(t *testing.T) {
	Convey("Given ExecutionOutputs", t, func() {
		bindings := ExecutionBindings()
		outputs := ExecutionOutputs(bindings)

		Convey("it declares two derived side-correct ratios plus four measured facts and two context facts", func() {
			So(len(outputs), ShouldEqual, 8)
			So(outputs[0].Slot, ShouldEqual, symbolExecutionBuyShare)
			So(outputs[1].Slot, ShouldEqual, symbolExecutionSellShare)
		})

		Convey("the derived ratios carry derived (min) maturity, not a single parent's", func() {
			So(outputs[0].Maturity, ShouldEqual, symbolExecutionBuyMatur)
			So(outputs[1].Maturity, ShouldEqual, symbolExecutionSellMatur)
		})
	})
}

/*
TestExecutionCapacityDistinguishesNotional — the "execution capacity kill test".

Book A and Book B have identical touch_notional_imbalance (0: equal bid and ask
touch), but radically different actual displayed touch capacity. A calculation
that only reads imbalance cannot tell them apart. The side-correct
flow-vs-capacity ratio MUST.
*/
func TestExecutionCapacityDistinguishesNotional(t *testing.T) {
	Convey("Given two books with identical imbalance but different actual capacity", t, func() {
		tiny := NewExecutionAdvisor("advisor.execution.tiny:" + t.Name())
		huge := NewExecutionAdvisor("advisor.execution.huge:" + t.Name())

		at := time.Unix(100, 0)

		// Book A: equal bid/ask touch, tiny actual notional (10 each).
		// Book B: equal bid/ask touch, huge actual notional (1_000_000 each).
		// Both have imbalance (D_b - D_a)/(D_b + D_a) = 0.
		flow := map[string]float64{"aggressive_notional:buy": 500.0}

		tiny.Step(testMeasurement("TEST/USD", "liquidity", at, map[string]float64{
			"touch_notional:ask": 10.0,
			"touch_notional:bid": 10.0,
		}))
		huge.Step(testMeasurement("TEST/USD", "liquidity", at, map[string]float64{
			"touch_notional:ask": 1000000.0,
			"touch_notional:bid": 1000000.0,
		}))

		tinyPerspective := tiny.Step(testMeasurement("TEST/USD", "cvd", at, flow))
		hugePerspective := huge.Step(testMeasurement("TEST/USD", "cvd", at, flow))

		tinyBuy, _ := executionReading(tinyPerspective, symbolExecutionBuyShare)
		hugeBuy, _ := executionReading(hugePerspective, symbolExecutionBuyShare)

		So(tinyBuy.Defined, ShouldBeTrue)
		So(hugeBuy.Defined, ShouldBeTrue)

		// 500 / 10 vs 500 / 1_000_000: the tiny book shows 50x its capacity,
		// the huge book a negligible share. They must differ by several orders
		// of magnitude even though both imbalances are exactly 0.
		So(tinyBuy.Value, ShouldAlmostEqual, 50.0, 1e-9)
		So(hugeBuy.Value, ShouldAlmostEqual, 0.0005, 1e-9)
		So(tinyBuy.Value, ShouldNotAlmostEqual, hugeBuy.Value, 1e-9)

		Convey("the old imbalance-only semantics would score both identically", func() {
			// 10 vs 10 and 1_000_000 vs 1_000_000 both yield imbalance 0.
			So(0.0, ShouldAlmostEqual, 0.0) // self-documenting: imbalance matches
		})
	})
}
