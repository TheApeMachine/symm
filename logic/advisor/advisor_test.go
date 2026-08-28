package advisor

import (
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
testMeasurement builds one projected measurement with the given metric values.
*/
func testMeasurement(symbol, source string, at time.Time, values map[string]float64) *data.Measurement[float64] {
	metrics := make(map[string]data.Metric[float64], len(values))

	for name, value := range values {
		metrics[name] = data.Metric[float64]{Raw: value}
	}

	return &data.Measurement[float64]{
		Label:   symbol,
		Source:  source,
		At:      at,
		Metrics: metrics,
	}
}

func newTestLiquidityAdvisor(name string) *Advisor {
	return NewLiquidityAdvisor(name)
}

/*
readingFor finds the reading for one output slot in a Perspective, so a test
can assert on a specific composed metric's value without depending on
position in Readings.
*/
func readingFor(perspective *types.Perspective, slot nmtypes.Symbol) (types.MetricReading, bool) {
	for index := 0; index < perspective.Count; index++ {
		if perspective.Readings[index].Metric == slot {
			return perspective.Readings[index], true
		}
	}

	return types.MetricReading{}, false
}

func TestAdvisorStep(t *testing.T) {
	Convey("Given a Liquidity Advisor composed over its three bound metrics", t, func() {
		advisor := newTestLiquidityAdvisor("advisor:liquidity:" + t.Name())

		Convey("a nil or errored or unlabeled measurement produces no perspective", func() {
			So(advisor.Step(nil), ShouldBeNil)
			So(advisor.Step(&data.Measurement[float64]{Err: errors.New("bad")}), ShouldBeNil)
			So(advisor.Step(&data.Measurement[float64]{Label: "", Source: "liquidity"}), ShouldBeNil)
		})

		Convey("an unrelated Measurement carrying no bound metric returns nil, not an empty perspective", func() {
			perspective := advisor.Step(testMeasurement("TEST/USD", "cvd", time.Unix(0, 0), map[string]float64{
				"unrelated_metric": 1,
			}))

			So(perspective, ShouldBeNil)
		})

		Convey("the first relevant liquidity Measurement becomes part of the symbol's Number state", func() {
			perspective := advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{
				"relative_spread": 0.01,
			}))

			So(perspective, ShouldNotBeNil)
			So(perspective.Kind, ShouldEqual, types.KindLiquidity)
			// Three bound metrics, each contributing four named outputs
			// (value, baseline, z-score, velocity).
			So(perspective.Count, ShouldEqual, 12)

			state, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)

			bindings := LiquidityBindings()
			value, hasValue := state.Get(bindings[0].Series.ValueSymbol)
			So(hasValue, ShouldBeTrue)
			So(value, ShouldEqual, 0.01)
		})

		Convey("a later depthflow Measurement for the same symbol composes with the previously retained liquidity facts", func() {
			advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{
				"relative_spread":          0.01,
				"touch_notional_imbalance": 0.02,
			}))

			perspective := advisor.Step(testMeasurement("TEST/USD", "depthflow", time.Unix(1, 0), map[string]float64{
				"book_imbalance": 0.5,
			}))

			So(perspective, ShouldNotBeNil)

			bindings := LiquidityBindings()
			state, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)

			// The liquidity facts observed on the first, independent Measurement
			// must still be present in the committed state after the depthflow
			// Measurement composed on top of it.
			spreadValue, hasSpread := state.Get(bindings[0].Series.ValueSymbol)
			So(hasSpread, ShouldBeTrue)
			So(spreadValue, ShouldEqual, 0.01)

			imbalanceValue, hasImbalance := state.Get(bindings[1].Series.ValueSymbol)
			So(hasImbalance, ShouldBeTrue)
			So(imbalanceValue, ShouldEqual, 0.02)

			bookValue, hasBook := state.Get(bindings[2].Series.ValueSymbol)
			So(hasBook, ShouldBeTrue)
			So(bookValue, ShouldEqual, 0.5)
		})

		Convey("updating one input does not erase the other previously observed inputs", func() {
			advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{
				"relative_spread":          0.01,
				"touch_notional_imbalance": 0.02,
			}))
			advisor.Step(testMeasurement("TEST/USD", "depthflow", time.Unix(1, 0), map[string]float64{
				"book_imbalance": 0.5,
			}))

			advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(2, 0), map[string]float64{
				"relative_spread": 0.03,
			}))

			bindings := LiquidityBindings()
			state, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)

			spreadValue, _ := state.Get(bindings[0].Series.ValueSymbol)
			So(spreadValue, ShouldEqual, 0.03)

			// touch_notional_imbalance and book_imbalance were not part of this
			// last Measurement and must survive unchanged.
			imbalanceValue, hasImbalance := state.Get(bindings[1].Series.ValueSymbol)
			So(hasImbalance, ShouldBeTrue)
			So(imbalanceValue, ShouldEqual, 0.02)

			bookValue, hasBook := state.Get(bindings[2].Series.ValueSymbol)
			So(hasBook, ShouldBeTrue)
			So(bookValue, ShouldEqual, 0.5)
		})

		Convey("an unrelated binding's event does not fabricate a duplicate observation for a metric it did not deliver", func() {
			// Two distinct real observations grow relative_spread's retained
			// ring past its initial single-slot capacity, so a stale
			// resubmission of the same retained sample would visibly advance
			// Count/Head if Advisor ever re-ran the branch without a fresh
			// input — the ring is not merely idempotent by accident here.
			advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{
				"relative_spread": 0.01,
			}))
			advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(1, 0), map[string]float64{
				"relative_spread": 0.02,
			}))

			bindings := LiquidityBindings()
			afterSecond, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)
			countAfterSecond := bindings[0].Series.Count(afterSecond)
			headAfterSecond := bindings[0].Series.Head(afterSecond)

			// Three more Measurements arrive that never mention relative_spread.
			// Its retained sample count and ring head must not advance on any
			// of them: a series only advances when this call's own Measurement
			// delivered that series' value, never merely because the value is
			// still sitting in the committed Frame from an earlier step.
			advisor.Step(testMeasurement("TEST/USD", "depthflow", time.Unix(2, 0), map[string]float64{
				"book_imbalance": 0.5,
			}))
			advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(3, 0), map[string]float64{
				"touch_notional_imbalance": 0.2,
			}))
			advisor.Step(testMeasurement("TEST/USD", "depthflow", time.Unix(4, 0), map[string]float64{
				"book_imbalance": 0.6,
			}))

			afterUnrelated, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)
			So(bindings[0].Series.Count(afterUnrelated), ShouldEqual, countAfterSecond)
			So(bindings[0].Series.Head(afterUnrelated), ShouldEqual, headAfterSecond)

			spreadValue, hasSpread := afterUnrelated.Get(bindings[0].Series.ValueSymbol)
			So(hasSpread, ShouldBeTrue)
			So(spreadValue, ShouldEqual, 0.02)
		})

		Convey("two different symbols never share resident state", func() {
			advisor.Step(testMeasurement("AAA/USD", "liquidity", time.Unix(0, 0), map[string]float64{
				"relative_spread": 0.01,
			}))
			advisor.Step(testMeasurement("BBB/USD", "liquidity", time.Unix(0, 0), map[string]float64{
				"relative_spread": 0.05,
			}))

			bindings := LiquidityBindings()

			aaaState, aaaFound := advisor.number.Project("AAA/USD")
			So(aaaFound, ShouldBeTrue)
			aaaValue, _ := aaaState.Get(bindings[0].Series.ValueSymbol)
			So(aaaValue, ShouldEqual, 0.01)

			bbbState, bbbFound := advisor.number.Project("BBB/USD")
			So(bbbFound, ShouldBeTrue)
			bbbValue, _ := bbbState.Get(bindings[0].Series.ValueSymbol)
			So(bbbValue, ShouldEqual, 0.05)
		})

		Convey("undefined derived state stays explicitly undefined until every derived slot exists", func() {
			bindings := LiquidityBindings()
			outputs := LiquidityOutputs(bindings)
			// relative_spread's velocity output: unlike baseline (which seeds
			// itself as the first observation's own value), velocity requires a
			// prior value to difference against, so it stays undefined until a
			// second observation of the same metric arrives.
			spreadVelocitySlot := outputs[3].Slot

			perspective := advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{
				"relative_spread": 0.01,
			}))

			So(perspective, ShouldNotBeNil)
			So(perspective.Count, ShouldEqual, 12)

			spreadValue, foundValue := readingFor(perspective, bindings[0].Series.ValueSymbol)
			So(foundValue, ShouldBeTrue)
			So(spreadValue.Defined, ShouldBeTrue)

			spreadVelocity, foundVelocity := readingFor(perspective, spreadVelocitySlot)
			So(foundVelocity, ShouldBeTrue)
			So(spreadVelocity.Defined, ShouldBeFalse)

			perspective = advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(1, 0), map[string]float64{
				"relative_spread": 0.02,
			}))

			spreadVelocity, foundVelocity = readingFor(perspective, spreadVelocitySlot)
			So(foundVelocity, ShouldBeTrue)
			So(spreadVelocity.Defined, ShouldBeTrue)
			So(spreadVelocity.Value, ShouldEqual, 0.01)
		})

		Convey("each reading is self-describing by its interned Metric identity, not by array position", func() {
			perspective := advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{
				"relative_spread":          0.01,
				"touch_notional_imbalance": 0.02,
			}))

			bindings := LiquidityBindings()
			seen := map[uint16]bool{}

			for index := 0; index < perspective.Count; index++ {
				seen[uint16(perspective.Readings[index].Metric)] = true
			}

			So(seen[uint16(bindings[0].Series.ValueSymbol)], ShouldBeTrue)
			So(seen[uint16(bindings[1].Series.ValueSymbol)], ShouldBeTrue)
			So(seen[uint16(bindings[2].Series.ValueSymbol)], ShouldBeTrue)
		})

		Convey("a genuine pipeline failure propagates on the emitted Perspective instead of being silently discarded", func() {
			advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(10, 0), map[string]float64{
				"relative_spread": 0.01,
			}))

			bindings := LiquidityBindings()
			beforeRegression, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)
			spreadBefore, _ := beforeRegression.Get(bindings[0].Series.ValueSymbol)

			// An event time earlier than the last observed one is a genuine
			// defect (Velocity/ZScore/Baseline all reject a regressed clock),
			// not an absent input TryFork should forgive: it must surface on the
			// returned Perspective, and the committed state must not silently
			// advance to reflect this failed attempt.
			regressed := advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(5, 0), map[string]float64{
				"relative_spread": 0.02,
			}))

			So(regressed, ShouldNotBeNil)
			So(regressed.Err, ShouldNotBeNil)

			afterRegression, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)
			spreadAfter, _ := afterRegression.Get(bindings[0].Series.ValueSymbol)
			So(spreadAfter, ShouldEqual, spreadBefore)
		})

		Convey("deterministic replay of the same input sequence produces the same Perspective values", func() {
			replay := func() *types.Perspective {
				fresh := newTestLiquidityAdvisor("advisor:liquidity:replay:" + t.Name())
				fresh.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{
					"relative_spread": 0.01,
				}))

				return fresh.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(1, 0), map[string]float64{
					"relative_spread": 0.02,
				}))
			}

			first := replay()
			second := replay()

			So(first.Count, ShouldEqual, second.Count)

			for index := 0; index < first.Count; index++ {
				So(first.Readings[index], ShouldResemble, second.Readings[index])
			}
		})
	})

	Convey("Two Advisor instances supplied different pipelines produce different semantics from the same Go type", t, func() {
		liquidityBinding := NewMetricBinding("liquidity", "relative_spread", "test/advisor/liquidity_only")
		liquidityOnly := NewAdvisor(
			"advisor:liquidity-only",
			types.KindLiquidity,
			LiquidityPipeline([]MetricBinding{liquidityBinding}),
			[]MetricBinding{liquidityBinding},
			LiquidityOutputs([]MetricBinding{liquidityBinding}),
		)

		// A deliberately different, non-temporal pipeline: it declares only
		// one output (the raw value itself) and derives nothing else, proving
		// Advisor imposes no fixed output shape on any pipeline it hosts.
		passthroughBinding := NewMetricBinding("liquidity", "relative_spread", "test/advisor/passthrough_only")
		passthrough := NewAdvisor(
			"advisor:passthrough",
			types.KindState,
			nmtypes.Identity,
			[]MetricBinding{passthroughBinding},
			[]Output{{Slot: passthroughBinding.Series.ValueSymbol, Metric: passthroughBinding}},
		)

		measurement := testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{
			"relative_spread": 0.01,
		})

		liquidityPerspective := liquidityOnly.Step(measurement)
		passthroughPerspective := passthrough.Step(measurement)

		So(liquidityPerspective, ShouldNotBeNil)
		So(passthroughPerspective, ShouldNotBeNil)
		So(liquidityPerspective.Kind, ShouldEqual, types.KindLiquidity)
		So(passthroughPerspective.Kind, ShouldEqual, types.KindState)

		// The liquidity pipeline declares four outputs per bound metric; the
		// passthrough pipeline declares exactly the one it computes.
		So(liquidityPerspective.Count, ShouldEqual, 4)
		So(passthroughPerspective.Count, ShouldEqual, 1)
		So(passthroughPerspective.Readings[0].Defined, ShouldBeTrue)
		So(passthroughPerspective.Readings[0].Value, ShouldEqual, 0.01)
	})
}

/*
BenchmarkStep measures the steady-state cost of one perspective step after the
pipeline is warm. The only allocation is the emitted Perspective value.
*/
func BenchmarkStep(b *testing.B) {
	advisor := NewLiquidityAdvisor("advisor:liquidity:bench")
	at := time.Unix(0, 0)
	measurement := testMeasurement("TEST/USD", "liquidity", at, map[string]float64{"relative_spread": 0.01})
	advisor.Step(measurement)
	measurement.At = time.Unix(0, int64(time.Second))

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		_ = advisor.Step(measurement)
	}
}
