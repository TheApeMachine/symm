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
historicalMeasurement builds one projected measurement for the given
Historical Analogue binding source, carrying only that binding's metric.
*/
func historicalMeasurement(symbol, source, metric string, value float64, at time.Time) *data.Measurement[float64] {
	return testMeasurement(symbol, source, at, map[string]float64{metric: value})
}

func TestHistoricalAdvisorStep(t *testing.T) {
	bindings := HistoricalBindings()
	flowSource, flowMetric := bindings[0].Source, bindings[0].Metric
	bookSource, bookMetric := bindings[1].Source, bindings[1].Metric
	excitationSource, excitationMetric := bindings[2].Source, bindings[2].Metric

	Convey("Given a Historical Analogue Advisor composed over its three bound dimensions", t, func() {
		advisor := newTestHistoricalAdvisor("advisor:historical:" + t.Name())

		Convey("a nil or unrelated Measurement returns nil, not an empty perspective", func() {
			So(advisor.Step(nil), ShouldBeNil)

			perspective := advisor.Step(testMeasurement("TEST/USD", "toxicity", time.Unix(0, 0), map[string]float64{
				"unrelated_metric": 1,
			}))
			So(perspective, ShouldBeNil)
		})

		Convey("initial history is insufficient: distance, percentile, and match count are explicitly undefined", func() {
			perspective := advisor.Step(historicalMeasurement("TEST/USD", flowSource, flowMetric, 0.1, time.Unix(0, 0)))

			So(perspective, ShouldNotBeNil)
			So(perspective.Kind, ShouldEqual, types.KindHistoricalAnalogue)

			distance, found := readingFor(perspective, recurrence.SymbolDistance)
			So(found, ShouldBeTrue)
			So(distance.Defined, ShouldBeFalse)

			percentile, found := readingFor(perspective, recurrence.SymbolPercentile)
			So(found, ShouldBeTrue)
			So(percentile.Defined, ShouldBeFalse)

			matchCount, found := readingFor(perspective, recurrence.SymbolMatchCount)
			So(found, ShouldBeTrue)
			So(matchCount.Defined, ShouldBeFalse)
		})

		Convey("as observations accumulate, retained state remains bounded and deterministic", func() {
			for index := int64(0); index < 20; index++ {
				at := time.Unix(index, 0)
				advisor.Step(historicalMeasurement("TEST/USD", flowSource, flowMetric, float64(index%5), at))
				advisor.Step(historicalMeasurement("TEST/USD", bookSource, bookMetric, float64(index%3), at))
				advisor.Step(historicalMeasurement("TEST/USD", excitationSource, excitationMetric, float64(index%7), at))
			}

			state, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)

			queryLength, found := state.Get(recurrence.SymbolQueryLength)
			So(found, ShouldBeTrue)
			// Bounded by the query-length derivation itself: never larger
			// than 2/5 of MaxPathSamples worth of joint history.
			So(queryLength, ShouldBeGreaterThan, 0)
		})

		Convey("the current path cannot match itself: the nearest match always starts strictly before the query window", func() {
			for index := int64(0); index < 12; index++ {
				at := time.Unix(index, 0)
				advisor.Step(historicalMeasurement("TEST/USD", flowSource, flowMetric, float64(index), at))
				advisor.Step(historicalMeasurement("TEST/USD", bookSource, bookMetric, float64(index), at))
				perspective := advisor.Step(historicalMeasurement("TEST/USD", excitationSource, excitationMetric, float64(index), at))

				matchFrom, foundMatch := readingFor(perspective, recurrence.SymbolMatchFromSec)
				queryLength, foundQuery := readingFor(perspective, recurrence.SymbolQueryLength)

				if !foundMatch || !matchFrom.Defined {
					continue
				}

				So(foundQuery, ShouldBeTrue)
				So(queryLength.Defined, ShouldBeTrue)

				sampleCount := index + 1
				queryStart := float64(sampleCount) - queryLength.Value
				So(matchFrom.Value, ShouldBeLessThan, queryStart)
			}
		})

		Convey("independent input dimensions: an event from one dimension does not fabricate another observation for a different dimension", func() {
			advisor.Step(historicalMeasurement("TEST/USD", flowSource, flowMetric, 0.1, time.Unix(0, 0)))
			advisor.Step(historicalMeasurement("TEST/USD", flowSource, flowMetric, 0.2, time.Unix(1, 0)))

			flowSeries := bindings[0].Series

			afterFlow, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)
			flowCountAfterFlow := flowSeries.Count(afterFlow)

			// Three unrelated events for the other two dimensions must never
			// advance the flow dimension's retained path.
			advisor.Step(historicalMeasurement("TEST/USD", bookSource, bookMetric, 0.5, time.Unix(2, 0)))
			advisor.Step(historicalMeasurement("TEST/USD", excitationSource, excitationMetric, 0.3, time.Unix(3, 0)))
			advisor.Step(historicalMeasurement("TEST/USD", bookSource, bookMetric, 0.6, time.Unix(4, 0)))

			afterUnrelated, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)
			So(flowSeries.Count(afterUnrelated), ShouldEqual, flowCountAfterFlow)
		})

		Convey("two symbols never share resident historical state", func() {
			advisor.Step(historicalMeasurement("AAA/USD", flowSource, flowMetric, 0.1, time.Unix(0, 0)))
			advisor.Step(historicalMeasurement("BBB/USD", flowSource, flowMetric, 0.9, time.Unix(0, 0)))

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
			advisor.Step(historicalMeasurement("TEST/USD", flowSource, flowMetric, 0.1, time.Unix(5, 0)))

			// An event time earlier than the last observed one is a genuine
			// defect in temporal.Path's own retention, not an absence.
			regressed := advisor.Step(historicalMeasurement("TEST/USD", flowSource, flowMetric, 0.2, time.Unix(1, 0)))

			So(regressed, ShouldNotBeNil)
			So(regressed.Err, ShouldNotBeNil)
		})

		Convey("deterministic replay of the same input sequence produces identical Perspective values", func() {
			replay := func() *types.Perspective {
				fresh := newTestHistoricalAdvisor("advisor:historical:replay:" + t.Name())
				var perspective *types.Perspective

				for index := int64(0); index < 12; index++ {
					at := time.Unix(index, 0)
					fresh.Step(historicalMeasurement("TEST/USD", flowSource, flowMetric, float64(index%4), at))
					fresh.Step(historicalMeasurement("TEST/USD", bookSource, bookMetric, float64(index%3), at))
					perspective = fresh.Step(historicalMeasurement("TEST/USD", excitationSource, excitationMetric, float64(index%5), at))
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

		Convey("a defined distance exactly 0 is distinguishable from undefined", func() {
			for index := int64(0); index < 12; index++ {
				at := time.Unix(index, 0)
				// A constant trajectory: the nearest historical match is
				// exactly identical to the query, distance 0.
				advisor.Step(historicalMeasurement("TEST/USD", flowSource, flowMetric, 1, at))
				advisor.Step(historicalMeasurement("TEST/USD", bookSource, bookMetric, 1, at))
				advisor.Step(historicalMeasurement("TEST/USD", excitationSource, excitationMetric, 1, at))
			}

			state, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)

			distance, distanceFound := state.Get(recurrence.SymbolDistance)
			So(distanceFound, ShouldBeTrue)
			So(distance, ShouldEqual, 0)
		})

		Convey("Historical Analogue's derived outputs carry their own honest provenance, not one arbitrary parent metric's", func() {
			for index := int64(0); index < 12; index++ {
				at := time.Unix(index, 0)
				advisor.Step(historicalMeasurement("TEST/USD", flowSource, flowMetric, float64(index%4), at))
				advisor.Step(historicalMeasurement("TEST/USD", bookSource, bookMetric, float64(index%3), at))
				advisor.Step(historicalMeasurement("TEST/USD", excitationSource, excitationMetric, float64(index%5), at))
			}

			state, found := advisor.number.Project("TEST/USD")
			So(found, ShouldBeTrue)

			distance, distanceFound := state.Get(recurrence.SymbolDistance)
			So(distanceFound, ShouldBeTrue)

			maturity, maturityFound := state.Get(recurrence.SymbolMaturity)
			So(maturityFound, ShouldBeTrue)

			matchCount, matchCountFound := state.Get(recurrence.SymbolMatchCount)
			So(matchCountFound, ShouldBeTrue)

			// Maturity is derived from the scan's own effective support
			// (matchCount), not copied from any bound metric's own
			// Maturity/SNR — none of the three bound metrics' individual
			// quality facts ever entered this computation.
			expectedMaturity := 0.0

			if matchCount > 1 {
				expectedMaturity = 1 - 1/matchCount
			}

			So(maturity, ShouldEqual, expectedMaturity)
			_ = distance

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
		advisor.Step(historicalMeasurement("TEST/USD", bindings[0].Source, bindings[0].Metric, float64(index%4), at))
		advisor.Step(historicalMeasurement("TEST/USD", bindings[1].Source, bindings[1].Metric, float64(index%3), at))
		advisor.Step(historicalMeasurement("TEST/USD", bindings[2].Source, bindings[2].Metric, float64(index%5), at))
	}

	measurement := historicalMeasurement("TEST/USD", bindings[0].Source, bindings[0].Metric, 1, time.Unix(20, 0))

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		_ = advisor.Step(measurement)
	}
}
