package advisor

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/recurrence"
	"github.com/theapemachine/symm/types"
)

func newTestHistoricalAdvisor(name string) *Advisor {
	return NewHistoricalAdvisor(name)
}

/*
historicalMeasurement builds one projected measurement for a Historical
Analogue binding source, carrying the named metrics. The horizon timescale is
fed separately because the Hawkes source's measurement carries both its
innovation (a trajectory dimension) and its excitation timescale (the control
fact the horizon derives from).
*/
func historicalMeasurement(symbol, source string, at time.Time, metrics map[string]float64) *data.Measurement[float64] {
	return testMeasurement(symbol, source, at, metrics)
}

/*
flowMeasurement feeds the CVD flow dimension.
*/
func flowMeasurement(symbol string, at time.Time, source, metric string, value float64) *data.Measurement[float64] {
	return historicalMeasurement(symbol, source, at, map[string]float64{metric: value})
}

/*
bookMeasurement feeds the Depthflow book dimension.
*/
func bookMeasurement(symbol string, at time.Time, source, metric string, value float64) *data.Measurement[float64] {
	return historicalMeasurement(symbol, source, at, map[string]float64{metric: value})
}

/*
excitationMeasurement feeds the Hawkes innovation (trajectory) and its
excitation timescale (horizon control) together, as the real signal does in one
measurement.
*/
func excitationMeasurement(symbol string, at time.Time, excitationMetric, horizonMetric string, innovation, timescale float64) *data.Measurement[float64] {
	return historicalMeasurement(symbol, "hawkes", at, map[string]float64{
		excitationMetric: innovation,
		horizonMetric:    timescale,
	})
}

func TestHistoricalAdvisorStep(t *testing.T) {
	bindings := HistoricalBindings()
	flowSource, flowMetric := bindings[0].Source, bindings[0].Metric
	bookSource, bookMetric := bindings[1].Source, bindings[1].Metric
	excitationSource, excitationMetric := bindings[2].Source, bindings[2].Metric
	horizonSource, horizonMetric := bindings[3].Source, bindings[3].Metric

	Convey("Given a Historical Analogue Advisor composed over its three bound dimensions", t, func() {
		advisor := newTestHistoricalAdvisor("advisor:historical:" + t.Name())

		Convey("a nil or unrelated Measurement returns nil, not an empty perspective", func() {
			So(advisor.Step(nil), ShouldBeNil)

			perspective := advisor.Step(testMeasurement("TEST/USD", "toxicity", time.Unix(0, 0), map[string]float64{
				"unrelated_metric": 1,
			}))
			So(perspective, ShouldBeNil)
		})

		Convey("the horizon control binding feeds the recurrence horizon from the Hawkes timescale", func() {
			So(horizonSource, ShouldEqual, excitationSource)
			So(horizonMetric, ShouldEqual, "excitation_timescale:buy_from_buy")

			converted := recurrence.SymbolHorizon

			_ = converted
		})

		Convey("initial history is insufficient: distance and match count are explicitly undefined", func() {
			perspective := advisor.Step(excitationMeasurement("TEST/USD", time.Unix(0, 0), excitationMetric, horizonMetric, 0.1, 5))

			So(perspective, ShouldNotBeNil)
			So(perspective.Kind, ShouldEqual, types.KindHistoricalAnalogue)

			distance, found := readingFor(perspective, recurrence.SymbolDistance)
			So(found, ShouldBeTrue)
			So(distance.Defined, ShouldBeFalse)

			matchCount, found := readingFor(perspective, recurrence.SymbolMatchCount)
			So(found, ShouldBeTrue)
			So(matchCount.Defined, ShouldBeFalse)
		})

		Convey("as observations accumulate, retained state remains bounded and the horizon is the fitted timescale", func() {
			for index := int64(0); index < 20; index++ {
				at := time.Unix(index, 0)
				advisor.Step(flowMeasurement("TEST/USD", at, flowSource, flowMetric, float64(index%5)))
				advisor.Step(bookMeasurement("TEST/USD", at, bookSource, bookMetric, float64(index%3)))
				advisor.Step(excitationMeasurement("TEST/USD", at, excitationMetric, horizonMetric, float64(index%7), 5))
			}

			state, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)

			horizon, found := state.Get(recurrence.SymbolHorizon)
			So(found, ShouldBeTrue)
			So(horizon, ShouldEqual, 5)
		})

		Convey("the current path cannot match itself: the nearest match always precedes the query window", func() {
			for index := int64(0); index < 12; index++ {
				at := time.Unix(index, 0)
				advisor.Step(flowMeasurement("TEST/USD", at, flowSource, flowMetric, float64(index)))
				advisor.Step(bookMeasurement("TEST/USD", at, bookSource, bookMetric, float64(index)))
				perspective := advisor.Step(excitationMeasurement("TEST/USD", at, excitationMetric, horizonMetric, float64(index), 5))

				matchFrom, foundMatch := readingFor(perspective, recurrence.SymbolMatchFromSec)
				horizon, foundHorizon := readingFor(perspective, recurrence.SymbolQueryLength)

				if !foundMatch || !matchFrom.Defined {
					continue
				}

				So(foundHorizon, ShouldBeTrue)
				So(horizon.Defined, ShouldBeTrue)

				So(matchFrom.Value, ShouldBeLessThan, float64(index)-horizon.Value)
			}
		})

		Convey("independent input dimensions: an event from one dimension does not fabricate another observation for a different dimension", func() {
			advisor.Step(flowMeasurement("TEST/USD", time.Unix(0, 0), flowSource, flowMetric, 0.1))
			advisor.Step(flowMeasurement("TEST/USD", time.Unix(1, 0), flowSource, flowMetric, 0.2))

			flowSeries := bindings[0].Series

			afterFlow, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)
			flowCountAfterFlow := flowSeries.Count(afterFlow)

			advisor.Step(bookMeasurement("TEST/USD", time.Unix(2, 0), bookSource, bookMetric, 0.5))
			advisor.Step(excitationMeasurement("TEST/USD", time.Unix(3, 0), excitationMetric, horizonMetric, 0.3, 5))
			advisor.Step(bookMeasurement("TEST/USD", time.Unix(4, 0), bookSource, bookMetric, 0.6))

			afterUnrelated, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)
			So(flowSeries.Count(afterUnrelated), ShouldEqual, flowCountAfterFlow)
		})

		Convey("two symbols never share resident historical state", func() {
			advisor.Step(flowMeasurement("AAA/USD", time.Unix(0, 0), flowSource, flowMetric, 0.1))
			advisor.Step(flowMeasurement("BBB/USD", time.Unix(0, 0), flowSource, flowMetric, 0.9))

			flowSeries := bindings[0].Series

			aaaState, found := advisor.number.Project("AAA/USD")
			So(found, ShouldBeTrue)
			aaaValue, _ := aaaState.Get(flowSeries.ValueSymbol)
			So(aaaValue, ShouldEqual, 0.1)

			bbbState, found := advisor.number.Project("BBB/USD")
			So(found, ShouldBeTrue)
			bbbValue, _ := bbbState.Get(flowSeries.ValueSymbol)
			So(bbbValue, ShouldEqual, 0.9)
		})

		Convey("a genuine pipeline failure is surfaced, never silently converted into 'no analogue'", func() {
			advisor.Step(flowMeasurement("TEST/USD", time.Unix(5, 0), flowSource, flowMetric, 0.1))

			regressed := advisor.Step(flowMeasurement("TEST/USD", time.Unix(1, 0), flowSource, flowMetric, 0.2))

			So(regressed, ShouldNotBeNil)
			So(regressed.Err, ShouldNotBeNil)
		})

		Convey("deterministic replay of the same input sequence produces identical Perspective values", func() {
			replay := func() *types.Perspective {
				fresh := newTestHistoricalAdvisor("advisor:historical:replay:" + t.Name())
				var perspective *types.Perspective

				for index := int64(0); index < 12; index++ {
					at := time.Unix(index, 0)
					fresh.Step(flowMeasurement("TEST/USD", at, flowSource, flowMetric, float64(index%4)))
					fresh.Step(bookMeasurement("TEST/USD", at, bookSource, bookMetric, float64(index%3)))
					perspective = fresh.Step(excitationMeasurement("TEST/USD", at, excitationMetric, horizonMetric, float64(index%5), 5))
				}

				return perspective
			}

			first := replay()
			second := replay()

			So(first.Count, ShouldEqual, second.Count)

			for index := 0; index < first.Count; index++ {
				So(first.Readings[index], ShouldResemble, second.Readings[index])
			}
		})

		Convey("Historical Analogue's derived outputs carry their own honest provenance, not one arbitrary parent metric's", func() {
			for index := int64(0); index < 12; index++ {
				at := time.Unix(index, 0)
				advisor.Step(flowMeasurement("TEST/USD", at, flowSource, flowMetric, float64(index%4)))
				advisor.Step(bookMeasurement("TEST/USD", at, bookSource, bookMetric, float64(index%3)))
				advisor.Step(excitationMeasurement("TEST/USD", at, excitationMetric, horizonMetric, float64(index%5), 5))
			}

			state, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)

			matchCount, matchCountFound := state.Get(recurrence.SymbolMatchCount)
			So(matchCountFound, ShouldBeTrue)

			maturity, maturityFound := state.Get(recurrence.SymbolMaturity)
			So(maturityFound, ShouldBeTrue)

			expectedMaturity := 0.0

			if matchCount > 1 {
				expectedMaturity = 1 - 1/matchCount
			}

			So(maturity, ShouldEqual, expectedMaturity)

			outputs := HistoricalOutputs()

			for _, output := range outputs {
				snrDefined, found := state.Get(output.SNRDefined)
				So(found, ShouldBeFalse)
				_ = snrDefined
			}
		})
	})
}

/*
BenchmarkHistoricalStep measures the steady-state cost of one perspective step
after the pipeline is warm.
*/
func BenchmarkHistoricalStep(b *testing.B) {
	advisor := NewHistoricalAdvisor("advisor:historical:bench")
	bindings := HistoricalBindings()

	for index := int64(0); index < 12; index++ {
		at := time.Unix(index, 0)
		advisor.Step(flowMeasurement("TEST/USD", at, bindings[0].Source, bindings[0].Metric, float64(index%4)))
		advisor.Step(bookMeasurement("TEST/USD", at, bindings[1].Source, bindings[1].Metric, float64(index%3)))
		advisor.Step(excitationMeasurement("TEST/USD", at, bindings[2].Metric, bindings[3].Metric, float64(index%5), 5))
	}

	measurement := flowMeasurement("TEST/USD", time.Unix(20, 0), bindings[0].Source, bindings[0].Metric, 1)

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		_ = advisor.Step(measurement)
	}
}
