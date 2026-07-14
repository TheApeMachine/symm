package types

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
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
		graph.AddNode(older)
		graph.AddNode(newer)

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
		graph.AddNode(first)
		graph.AddNode(&second)

		graph.Compose()

		Convey("It should record support without inventing temporal order", func() {
			So(edgeTypes(graphEdges(graph)), ShouldResemble, []EdgeType{Supports})
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
		graph.AddNode(first)
		graph.AddNode(&second)

		graph.Compose()

		Convey("It should not equate unnamed observables", func() {
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
		graph.AddNode(first)
		graph.AddNode(&second)

		graph.Compose()

		Convey("It should not claim directional agreement", func() {
			So(graphEdges(graph), ShouldBeEmpty)
		})
	})

	Convey("Given a busy chronological stream of one observable", t, func() {
		graph := NewGraph("BTC/USD")
		normalized := 0.5

		for index := range 1_000 {
			at := time.Unix(int64(index+1), 0)
			graph.AddNode(&Measurement{
				Source: SourceHawkes, Stream: Hawkes, Metric: MetricStrength,
				Subject: SubjectTradeArrivals, Symbol: "BTC/USD",
				At: at, ObservedFrom: at.Add(-time.Second),
				Horizon: time.Second, Unit: UnitDimensionless,
				Normalized: &normalized,
				Validity:   MeasurementValidity{State: ValidityValid},
				Scale:      ScaleReference{Kind: ScaleObservationWindow},
			})
		}

		graph.Compose()

		Convey("Then direct evidence remains connected without a transitive closure", func() {
			So(graph.Nodes().Len(), ShouldEqual, 1_000)
			So(graphEdges(graph), ShouldHaveLength, 999)
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
			graph.AddNode(measurement)
		}

		graph.Compose()
	}
}
