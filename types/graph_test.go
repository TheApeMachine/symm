package types

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGraphAddNode(t *testing.T) {
	Convey("Given a symbol-local graph", t, func() {
		graph := NewGraph("BTC/USD")
		at := time.Unix(10, 0)
		measurement := &Measurement{
			Stream:  Hawkes,
			Metric:  MetricArrivalRate,
			Subject: SubjectTradeArrivals,
			Side:    SideBuy,
			Symbol:  "BTC/USD",
			At:      at,
		}

		Convey("When the same measurement is added twice", func() {
			first := graph.AddNode(measurement)
			second := graph.AddNode(measurement)

			Convey("Then only one node is retained", func() {
				So(first, ShouldBeTrue)
				So(second, ShouldBeFalse)
				So(graph.Nodes, ShouldHaveLength, 1)
				So(graph.At, ShouldEqual, at)
			})
		})

		Convey("When the same metric is observed at a later time", func() {
			later := *measurement
			later.At = at.Add(time.Second)

			first := graph.AddNode(measurement)
			second := graph.AddNode(&later)

			Convey("Then both evidence instances are retained", func() {
				So(first, ShouldBeTrue)
				So(second, ShouldBeTrue)
				So(graph.Nodes, ShouldHaveLength, 2)
			})
		})

		Convey("When pointer-backed estimator values change after insertion", func() {
			normalized := 0.5
			measurement.Normalized = &normalized
			measurement.Uncertainty = &MeasurementUncertainty{
				Lower: 0.4, Upper: 0.6,
			}
			graph.AddNode(measurement)

			normalized = 0.9
			measurement.Uncertainty.Lower = 0.8

			Convey("Then retained evidence remains immutable", func() {
				So(*graph.Nodes[0].Measurement.Normalized, ShouldEqual, 0.5)
				So(graph.Nodes[0].Measurement.Uncertainty.Lower, ShouldEqual, 0.4)
			})
		})

		Convey("When a foreign symbol measurement is added", func() {
			foreign := &Measurement{
				Stream: Hawkes,
				Metric: MetricArrivalRate,
				Symbol: "ETH/USD",
				At:     at,
			}

			added := graph.AddNode(foreign)

			Convey("Then it is rejected", func() {
				So(added, ShouldBeFalse)
				So(graph.Nodes, ShouldBeEmpty)
			})
		})
	})
}

func TestGraphRelate(t *testing.T) {
	Convey("Given two measurement nodes", t, func() {
		graph := NewGraph("BTC/USD")
		at := time.Unix(20, 0)
		from := &Measurement{
			Stream:  Hawkes,
			Metric:  MetricArrivalRate,
			Subject: SubjectTradeArrivals,
			Side:    SideBuy,
			Symbol:  "BTC/USD",
			At:      at,
		}
		to := &Measurement{
			Stream:  PumpDump,
			Metric:  MetricEventCount,
			Subject: SubjectTradeArrivals,
			Side:    SideNone,
			Symbol:  "BTC/USD",
			At:      at,
		}

		graph.AddNode(from)
		graph.AddNode(to)

		Convey("When they are linked with temporal context", func() {
			linked := graph.Relate(
				measurementKey(from),
				measurementKey(to),
				Supports,
				at,
				at.Add(-time.Second),
			)

			Convey("Then the edge retains provenance", func() {
				So(linked, ShouldBeTrue)
				So(graph.Edges, ShouldHaveLength, 1)
				So(graph.Edges[0].Type, ShouldEqual, Supports)
				So(graph.Edges[0].ObservedFrom, ShouldEqual, at.Add(-time.Second))
			})
		})
	})
}

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
			So(graph.Edges, ShouldHaveLength, 4)
			So(edgeTypes(graph.Edges), ShouldResemble, []EdgeType{
				Contradicts, Leads, Lags, Stale,
			})
			So(graph.Edges[0].At, ShouldEqual, newer.At)
			So(graph.Edges[0].ObservedFrom, ShouldEqual, older.ObservedFrom)
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
			So(edgeTypes(graph.Edges), ShouldResemble, []EdgeType{Supports})
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
			So(graph.Edges, ShouldBeEmpty)
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
			So(graph.Edges, ShouldBeEmpty)
		})
	})
}

func edgeTypes(edges []Edge) []EdgeType {
	types := make([]EdgeType, 0, len(edges))

	for _, edge := range edges {
		types = append(types, edge.Type)
	}

	return types
}

func BenchmarkGraphAddNode(b *testing.B) {
	graph := NewGraph("BTC/USD")
	measurement := &Measurement{
		Stream:  Hawkes,
		Metric:  MetricArrivalRate,
		Subject: SubjectTradeArrivals,
		Side:    SideBuy,
		Symbol:  "BTC/USD",
		At:      time.Unix(1, 0),
	}

	for b.Loop() {
		graph.AddNode(measurement)
	}
}

func BenchmarkGraphCompose(b *testing.B) {
	normalized := 0.5
	measurements := make([]*Measurement, 16)

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
