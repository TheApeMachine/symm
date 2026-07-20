package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestEvidenceComposerAddMeasurements proves batch staging preserves exactly the
newest direct relationship instead of allocating every historical reading.
*/
func TestEvidenceComposerAddMeasurements(t *testing.T) {
	Convey("Given a long history for one typed observable", t, func() {
		graph := NewGraph("BTC/USD")
		measurements := make([]*Measurement, 1000)
		normalized := 0.5

		for index := range measurements {
			measurements[index] = &Measurement{
				Source:     SourceHawkes,
				Stream:     Hawkes,
				Metric:     MetricStrength,
				Subject:    SubjectTradeArrivals,
				Symbol:     "BTC/USD",
				At:         time.Unix(int64(index+1), 0),
				Unit:       UnitDimensionless,
				Normalized: &normalized,
				Validity: MeasurementValidity{
					State: ValidityValid,
				},
			}
		}

		Convey("When the evidence batch is staged and composed", func() {
			err := graph.Evidence.AddMeasurements(measurements)
			graph.Compose()
			nodes := graph.Nodes()

			Convey("Then only the newest neighboring readings define the graph", func() {
				So(err, ShouldBeNil)
				So(nodes, ShouldHaveLength, 2)
				So(graph.Edges(), ShouldHaveLength, 3)
				So(edgeTypes(graph.Edges()), ShouldContain, Redundant)
				So(edgeTypes(graph.Edges()), ShouldContain, Leads)
				So(edgeTypes(graph.Edges()), ShouldContain, Lags)

				for _, node := range nodes {
					So(node.Measurement.At.Before(time.Unix(999, 0)), ShouldBeFalse)
				}
			})
		})
	})
}

/*
BenchmarkEvidenceComposerAddMeasurements measures history preselection plus the
real graph node construction path for a burst-shaped observable history.
*/
func BenchmarkEvidenceComposerAddMeasurements(b *testing.B) {
	measurements := make([]*Measurement, 1000)
	normalized := 0.5

	for index := range measurements {
		measurements[index] = &Measurement{
			Source:     SourceHawkes,
			Stream:     Hawkes,
			Metric:     MetricStrength,
			Subject:    SubjectTradeArrivals,
			Symbol:     "BTC/USD",
			At:         time.Unix(int64(index+1), 0),
			Unit:       UnitDimensionless,
			Normalized: &normalized,
			Validity:   MeasurementValidity{State: ValidityValid},
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		graph := NewGraph("BTC/USD")

		if err := graph.Evidence.AddMeasurements(measurements); err != nil {
			b.Fatal(err)
		}

		graph.Compose()
	}
}
