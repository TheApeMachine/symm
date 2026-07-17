package types

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
)

func TestGraphCompose(t *testing.T) {
	Convey("Given comparable observations from disjoint evidence intervals", t, func() {
		graph := NewGraph("BTC/USD")
		positive := 0.5
		negative := -0.4
		older := &Measurement{
			Source: SourceHawkes, Stream: Hawkes, Metric: MetricStrength,
			Subject: SubjectTradeArrivals, Symbol: "BTC/USD",
			At: time.Unix(2, 0), ObservedFrom: time.Unix(1, 0),
			Horizon: time.Second, Unit: UnitDimensionless,
			Normalized: &positive,
			Validity:   MeasurementValidity{State: ValidityValid},
			Scale:      ScaleReference{Kind: ScaleObservationWindow},
		}
		newer := &Measurement{
			Source: SourcePumpDump, Stream: PumpDump, Metric: MetricStrength,
			Subject: SubjectTradeArrivals, Symbol: "BTC/USD",
			At: time.Unix(4, 0), ObservedFrom: time.Unix(3, 0),
			Horizon: time.Second, Unit: UnitDimensionless,
			Normalized: &negative,
			Validity:   MeasurementValidity{State: ValidityValid},
			Scale:      ScaleReference{Kind: ScaleObservationWindow},
		}
		So(graph.AddNode(older), ShouldBeNil)
		So(graph.AddNode(newer), ShouldBeNil)

		graph.Compose()

		Convey("It should preserve contradiction, lead, lag, and staleness separately", func() {
			edges := graphEdges(graph)
			So(edges, ShouldHaveLength, 4)
			So(edgeTypes(edges), ShouldContain, Contradicts)
			So(edgeTypes(edges), ShouldContain, Leads)
			So(edgeTypes(edges), ShouldContain, Lags)
			So(edgeTypes(edges), ShouldContain, Stale)

			for _, edge := range edges {
				So(edge.At, ShouldEqual, newer.At)
				So(edge.ObservedFrom, ShouldEqual, older.ObservedFrom)
			}
		})
	})

	Convey("Given simultaneous same-direction observations", t, func() {
		graph := NewGraph("BTC/USD")
		firstValue := 0.2
		secondValue := 0.8
		first := &Measurement{
			Source: SourceHawkes, Stream: Hawkes, Metric: MetricStrength,
			Subject: SubjectTradeArrivals, Symbol: "BTC/USD",
			At: time.Unix(2, 0), ObservedFrom: time.Unix(1, 0),
			Unit: UnitDimensionless, Normalized: &firstValue,
			Validity: MeasurementValidity{State: ValidityValid},
			Scale:    ScaleReference{Kind: ScaleObservationWindow},
		}
		second := *first
		second.Source = SourcePumpDump
		second.Stream = PumpDump
		second.Normalized = &secondValue
		So(graph.AddNode(first), ShouldBeNil)
		So(graph.AddNode(&second), ShouldBeNil)

		graph.Compose()

		Convey("It should record support without inventing temporal order", func() {
			So(edgeTypes(graphEdges(graph)), ShouldResemble, []EdgeType{Supports})
		})
	})

	Convey("Given repeated observations with identical normalized evidence", t, func() {
		graph := NewGraph("BTC/USD")
		normalized := 0.4
		older := &Measurement{
			Source: SourceHawkes, Stream: Hawkes, Metric: MetricStrength,
			Subject: SubjectTradeArrivals, Symbol: "BTC/USD",
			At: time.Unix(1, 0), Unit: UnitDimensionless,
			Normalized: &normalized,
			Validity:   MeasurementValidity{State: ValidityValid},
		}
		newer := *older
		newer.At = time.Unix(2, 0)
		So(graph.AddNode(older), ShouldBeNil)
		So(graph.AddNode(&newer), ShouldBeNil)

		graph.Compose()

		Convey("It should retain redundancy as a distinct relationship", func() {
			So(edgeTypes(graphEdges(graph)), ShouldContain, Redundant)
		})
	})

	Convey("Given measurements without a typed subject", t, func() {
		graph := NewGraph("BTC/USD")
		normalized := 0.5
		first := &Measurement{
			Source: SourceHawkes, Stream: Hawkes, Metric: MetricStrength,
			Symbol: "BTC/USD", At: time.Unix(1, 0),
			Unit: UnitDimensionless, Normalized: &normalized,
			Validity: MeasurementValidity{State: ValidityValid},
		}
		second := *first
		second.Source = SourcePumpDump
		second.Stream = PumpDump
		firstErr := graph.AddNode(first)
		secondErr := graph.AddNode(&second)

		graph.Compose()

		Convey("It should not equate unnamed observables", func() {
			So(firstErr, ShouldBeNil)
			So(secondErr, ShouldBeNil)
			So(graphEdges(graph), ShouldBeEmpty)
		})
	})

	Convey("Given non-finite normalized evidence", t, func() {
		graph := NewGraph("BTC/USD")
		finite := 0.5
		nonFinite := math.NaN()
		first := &Measurement{
			Source: SourceHawkes, Stream: Hawkes, Metric: MetricStrength,
			Subject: SubjectTradeArrivals, Symbol: "BTC/USD",
			At: time.Unix(1, 0), Unit: UnitDimensionless,
			Normalized: &finite,
			Validity:   MeasurementValidity{State: ValidityValid},
		}
		second := *first
		second.Source = SourcePumpDump
		second.Stream = PumpDump
		second.Normalized = &nonFinite
		So(graph.AddNode(first), ShouldBeNil)
		err := graph.AddNode(&second)

		graph.Compose()

		Convey("It should reject the invalid value before composing", func() {
			So(err, ShouldNotBeNil)
			So(errnie.IsValidation(err), ShouldBeTrue)
			So(graphEdges(graph), ShouldBeEmpty)
		})
	})

	Convey("Given a busy chronological stream of one observable", t, func() {
		graph := NewGraph("BTC/USD")
		normalized := 0.5

		for index := range 1_000 {
			at := time.Unix(int64(index+1), 0)
			err := graph.AddNode(&Measurement{
				Source: SourceHawkes, Stream: Hawkes, Metric: MetricStrength,
				Subject: SubjectTradeArrivals, Symbol: "BTC/USD",
				At: at, ObservedFrom: at.Add(-time.Second),
				Horizon: time.Second, Unit: UnitDimensionless,
				Normalized: &normalized,
				Validity:   MeasurementValidity{State: ValidityValid},
				Scale:      ScaleReference{Kind: ScaleObservationWindow},
			})
			So(err, ShouldBeNil)
		}

		graph.Compose()

		Convey("Then only the current direct relationship remains on the live graph", func() {
			So(graph.Nodes(), ShouldHaveLength, 2)
			So(graphEdges(graph), ShouldHaveLength, 1)
			So(graph.Nodes()[0].Measurement.At.Unix(), ShouldBeGreaterThanOrEqualTo, int64(999))
			So(graph.Nodes()[1].Measurement.At.Unix(), ShouldBeGreaterThanOrEqualTo, int64(999))
		})
	})
}

func edgeTypes(edges []*Edge) []EdgeType {
	types := make([]EdgeType, 0, len(edges))

	for _, edge := range edges {
		types = append(types, edge.Type)
	}

	return types
}

func BenchmarkGraphCompose(b *testing.B) {
	normalized := 0.5
	measurements := make([]*Measurement, 1_000)

	for index := range measurements {
		measurements[index] = &Measurement{
			Source: SourceHawkes, Stream: Hawkes, Metric: MetricStrength,
			Subject: SubjectTradeArrivals, Symbol: "BTC/USD",
			At:           time.Unix(int64(index+1), 0),
			ObservedFrom: time.Unix(int64(index), 0),
			Horizon:      time.Second, Unit: UnitDimensionless,
			Normalized: &normalized,
			Validity:   MeasurementValidity{State: ValidityValid},
			Scale:      ScaleReference{Kind: ScaleObservationWindow},
		}
	}

	for b.Loop() {
		graph := NewGraph("BTC/USD")

		for _, measurement := range measurements {
			if err := graph.AddNode(measurement); err != nil {
				b.Fatal(err)
			}
		}

		graph.Compose()
	}
}
