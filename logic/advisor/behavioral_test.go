package advisor

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/types"
)

/*
TestFlowBookAlignmentSign is the cross-metric behavioral test: flow_book_alignment
= signed_net_fraction * book_imbalance keeps the exact joint sign, and changing
an unrelated metric does not move it.
*/
func TestFlowBookAlignmentSign(t *testing.T) {
	Convey("Given a Flow advisor", t, func() {
		advisor := NewFlowAdvisor("advisor.flow.behavioral:" + t.Name())
		at := time.Unix(100, 0)

		Convey("positive signed flow × positive book imbalance is positive", func() {
			perspective := advisor.Step(testMeasurement("TEST/USD", "cvd", at, map[string]float64{
				"signed_net_fraction": 0.5,
			}))
			So(perspective, ShouldNotBeNil)

			perspective = advisor.Step(testMeasurement("TEST/USD", "depthflow", at, map[string]float64{
				"book_imbalance": 0.4,
			}))

			alignment, found := readingFor(perspective, symbolFlowBookAlignment)
			So(found, ShouldBeTrue)
			So(alignment.Defined, ShouldBeTrue)
			So(alignment.Value, ShouldAlmostEqual, 0.2, 1e-9)
		})

		Convey("positive signed flow × negative book imbalance is negative", func() {
			advisor.Step(testMeasurement("TEST/USD", "cvd", at, map[string]float64{
				"signed_net_fraction": 0.5,
			}))

			perspective := advisor.Step(testMeasurement("TEST/USD", "depthflow", at, map[string]float64{
				"book_imbalance": -0.4,
			}))

			alignment, found := readingFor(perspective, symbolFlowBookAlignment)
			So(found, ShouldBeTrue)
			So(alignment.Value, ShouldAlmostEqual, -0.2, 1e-9)
		})

		Convey("changing an unrelated metric does not alter flow_book_alignment", func() {
			advisor.Step(testMeasurement("TEST/USD", "cvd", at, map[string]float64{
				"signed_net_fraction": 0.5,
			}))
			advisor.Step(testMeasurement("TEST/USD", "depthflow", at, map[string]float64{
				"book_imbalance": 0.4,
			}))

			before, foundBefore := readingFor(advisor.Step(testMeasurement("TEST/USD", "cvd", at, map[string]float64{"signed_net_fraction": 0.5})), symbolFlowBookAlignment)
			So(foundBefore, ShouldBeTrue)

			// A bound-but-irrelevant CVD fact (net notional rate) must not move
			// flow_book_alignment, which depends only on signed_net_fraction and
			// book_imbalance.
			afterPerspective := advisor.Step(testMeasurement("TEST/USD", "cvd", at, map[string]float64{"net_notional_rate": 123.0}))
			So(afterPerspective, ShouldNotBeNil)
			after, foundAfter := readingFor(afterPerspective, symbolFlowBookAlignment)
			So(foundAfter, ShouldBeTrue)

			So(after.Value, ShouldAlmostEqual, before.Value, 1e-9)
		})
	})
}

/*
TestExecutionArrivalPerCapacity proves the Hawkes/capacity conditioning:
fixed Hawkes buy intensity with smaller ask touch produces a larger
buy_arrival_per_ask_touch, and sell uses bid capacity, never ask.
*/
func TestExecutionArrivalPerCapacity(t *testing.T) {
	Convey("Given an Execution advisor", t, func() {
		at := time.Unix(100, 0)

		Convey("smaller ask touch with fixed buy intensity raises buy arrival ratio", func() {
			small := NewExecutionAdvisor("advisor.execution.arrival.small:" + t.Name())
			small.Step(testMeasurement("TEST/USD", "liquidity", at, map[string]float64{"touch_notional:ask": 100.0}))
			smallPerspective := small.Step(testMeasurement("TEST/USD", "hawkes", at, map[string]float64{"conditional_intensity:buy": 50.0}))
			smallArrival, _ := executionReading(smallPerspective, symbolExecutionBuyArrival)

			large := NewExecutionAdvisor("advisor.execution.arrival.large:" + t.Name())
			large.Step(testMeasurement("TEST/USD", "liquidity", at, map[string]float64{"touch_notional:ask": 10000.0}))
			largePerspective := large.Step(testMeasurement("TEST/USD", "hawkes", at, map[string]float64{"conditional_intensity:buy": 50.0}))
			largeArrival, _ := executionReading(largePerspective, symbolExecutionBuyArrival)

			So(smallArrival.Defined, ShouldBeTrue)
			So(largeArrival.Defined, ShouldBeTrue)
			So(smallArrival.Value, ShouldBeGreaterThan, largeArrival.Value)
		})

		Convey("sell arrival uses bid capacity, never ask", func() {
			advisor := NewExecutionAdvisor("advisor.execution.arrival.side:" + t.Name())
			advisor.Step(testMeasurement("TEST/USD", "liquidity", at, map[string]float64{
				"touch_notional:ask": 1000.0, // ignored for sell
				"touch_notional:bid": 500.0,  // the sell denominator
			}))

			perspective := advisor.Step(testMeasurement("TEST/USD", "hawkes", at, map[string]float64{"conditional_intensity:sell": 100.0}))
			sellArrival, _ := executionReading(perspective, symbolExecutionSellArrival)

			So(sellArrival.Defined, ShouldBeTrue)
			So(sellArrival.Value, ShouldAlmostEqual, 0.2, 1e-9) // 100 / 500
		})
	})
}

/*
TestPerspectiveStoreLatest proves the advisor.Store latest-by-key semantics:
latest sequence wins and an errored Perspective never replaces a valid latest.
*/
func TestPerspectiveStoreLatest(t *testing.T) {
	Convey("Given a perspective store", t, func() {
		store := NewStore()
		key := types.PerspectiveKey{Symbol: "TEST/USD", Kind: types.KindFlow}

		first := &types.Perspective{Symbol: "TEST/USD", Kind: types.KindFlow, Sequence: 1}
		first.Readings[0] = types.MetricReading{Value: 1.0, Defined: true}
		first.Count = 1

		store.Put(first)

		Convey("the first is readable", func() {
			got, found := store.Latest(key)
			So(found, ShouldBeTrue)
			So(got.Sequence, ShouldEqual, 1)
		})

		Convey("a newer sequence wins", func() {
			second := &types.Perspective{Symbol: "TEST/USD", Kind: types.KindFlow, Sequence: 2}
			second.Count = 1
			store.Put(second)

			got, found := store.Latest(key)
			So(found, ShouldBeTrue)
			So(got.Sequence, ShouldEqual, 2)
		})

		Convey("an errored Perspective never replaces a valid latest", func() {
			errone := &types.Perspective{Symbol: "TEST/USD", Kind: types.KindFlow, Sequence: 3, Err: errAny()}
			store.Put(errone)

			got, found := store.Latest(key)
			So(found, ShouldBeTrue)
			So(got.Sequence, ShouldNotEqual, 3)
		})
	})
}

func errAny() error {
	return &errTest{}
}

type errTest struct{}

func (errTest) Error() string { return "test error" }
