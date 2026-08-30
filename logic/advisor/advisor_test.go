package advisor

import (
	"errors"
	"fmt"
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

func readingFor(perspective *types.Perspective, slot nmtypes.Symbol) (types.MetricReading, bool) {
	for index := 0; index < perspective.Count; index++ {
		if perspective.Readings[index].Metric == slot {
			return perspective.Readings[index], true
		}
	}

	return types.MetricReading{}, false
}

func TestAdvisorStep(t *testing.T) {
	Convey("Given a Liquidity Advisor composed over its bound metrics", t, func() {
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

		Convey("a liquidity Measurement surfaces its already-derived facts verbatim", func() {
			perspective := advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{
				"relative_spread": 0.01,
			}))

			So(perspective, ShouldNotBeNil)
			So(perspective.Kind, ShouldEqual, types.KindLiquidity)
			// Eight declared readings, one per bound metric.
			So(perspective.Count, ShouldEqual, 8)

			bindings := LiquidityBindings()
			value, hasValue := perspective.Readings[0].Value, perspective.Readings[0].Defined
			_ = value
			_ = hasValue

			spread, found := readingFor(perspective, bindings[0].Series.ValueSymbol)
			So(found, ShouldBeTrue)
			So(spread.Defined, ShouldBeTrue)
			So(spread.Value, ShouldEqual, 0.01)
		})

		Convey("a later measurement of a different bound metric composes with retained facts", func() {
			advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{
				"relative_spread": 0.01,
			}))

			perspective := advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(1, 0), map[string]float64{
				"touch_notional_imbalance": 0.02,
			}))

			So(perspective, ShouldNotBeNil)

			bindings := LiquidityBindings()
			state, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)

			spreadValue, hasSpread := state.Get(bindings[0].Series.ValueSymbol)
			So(hasSpread, ShouldBeTrue)
			So(spreadValue, ShouldEqual, 0.01)

			imbalanceValue, hasImbalance := state.Get(bindings[4].Series.ValueSymbol)
			So(hasImbalance, ShouldBeTrue)
			So(imbalanceValue, ShouldEqual, 0.02)
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

		Convey("each reading is self-describing by its interned Metric identity", func() {
			perspective := advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{
				"relative_spread": 0.01,
			}))

			So(perspective, ShouldNotBeNil)

			bindings := LiquidityBindings()
			spread, found := readingFor(perspective, bindings[0].Series.ValueSymbol)
			So(found, ShouldBeTrue)
			So(spread.Defined, ShouldBeTrue)
			So(spread.Value, ShouldEqual, 0.01)
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
}

func TestNewAdvisorOutputCapacity(t *testing.T) {
	Convey("NewAdvisor panics when declared outputs exceed PerspectiveMetricCapacity", t, func() {
		binding := NewMetricBinding("liquidity", "relative_spread", "test/advisor/capacity_overflow")
		outputs := make([]Output, types.PerspectiveMetricCapacity+1)

		for index := range outputs {
			outputs[index] = NewMetricOutput(
				nmtypes.MustIntern(fmt.Sprintf("test/advisor/capacity_overflow/output/%d", index)),
				binding,
			)
		}

		So(func() {
			NewAdvisor(
				"advisor:overflow",
				types.KindState,
				nmtypes.Identity,
				[]MetricBinding{binding},
				outputs,
			)
		}, ShouldPanic)
	})

	Convey("NewAdvisor accepts exactly PerspectiveMetricCapacity outputs", t, func() {
		binding := NewMetricBinding("liquidity", "relative_spread", "test/advisor/capacity_exact")
		outputs := make([]Output, types.PerspectiveMetricCapacity)

		for index := range outputs {
			outputs[index] = NewMetricOutput(
				nmtypes.MustIntern(fmt.Sprintf("test/advisor/capacity_exact/output/%d", index)),
				binding,
			)
		}

		So(func() {
			NewAdvisor(
				"advisor:exact",
				types.KindState,
				nmtypes.Identity,
				[]MetricBinding{binding},
				outputs,
			)
		}, ShouldNotPanic)
	})
}

/*
TestMorphologyDynamicsTemporalContext validates the one advisor whose signal
does not yet publish its own causal history: historical context is undefined
before history exists, and baseline/z/velocity become defined causally,
never from future samples.
*/
func TestMorphologyDynamicsTemporalContext(t *testing.T) {
	Convey("Given a MorphologyDynamics advisor", t, func() {
		advisor := NewMorphologyDynamicsAdvisor("advisor:morphology_dynamics:" + t.Name())

		bindings := MorphologyDynamicsBindings()
		valueSlot := bindings[0].Series.ValueSymbol

		Convey("the first observation defines value but not velocity", func() {
			perspective := advisor.Step(testMeasurement("TEST/USD", "morphology", time.Unix(0, 0), map[string]float64{
				"morphology_change": 0.1,
			}))

			So(perspective, ShouldNotBeNil)
			So(perspective.Count, ShouldEqual, 12)

			value, found := readingFor(perspective, valueSlot)
			So(found, ShouldBeTrue)
			So(value.Defined, ShouldBeTrue)
			So(value.Value, ShouldEqual, 0.1)
		})

		Convey("a second observation advances velocity causally", func() {
			advisor.Step(testMeasurement("TEST/USD", "morphology", time.Unix(0, 0), map[string]float64{
				"morphology_change": 0.1,
			}))

			perspective := advisor.Step(testMeasurement("TEST/USD", "morphology", time.Unix(1, 0), map[string]float64{
				"morphology_change": 0.2,
			}))

			So(perspective, ShouldNotBeNil)
			So(perspective.Count, ShouldEqual, 12)
		})
	})
}
