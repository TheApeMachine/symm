package leadlag

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/types"
)

func TestMeasurement(t *testing.T) {
	Convey("Given a leaderless cross-section", t, func() {
		signal := NewSignal(context.Background(), types.NewThesis(context.Background(), nil))
		at := time.Unix(1_700_007_000, 0).UTC()
		signal.section.ObservePrice("AAA/USD", 100, at)

		measurement := signal.measurement("AAA/USD", at)

		Convey("It should publish explicit provisional evidence", func() {
			So(measurement.Peer, ShouldBeBlank)
			So(measurement.Metrics[string(types.MetricInefficient)].Normalized, ShouldNotBeNil)
			So(*measurement.Metrics[string(types.MetricInefficient)].Normalized, ShouldEqual, 0.0)
			So(measurement.Metrics[string(types.MetricSignedLagDirection)].Raw, ShouldEqual, 0.0)
		})
	})
}

func BenchmarkMeasurement(benchmark *testing.B) {
	signal := NewSignal(context.Background(), types.NewThesis(context.Background(), nil))
	at := time.Unix(1_700_007_000, 0).UTC()
	signal.section.ObservePrice("AAA/USD", 100, at)
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal.measurement("AAA/USD", at)
	}
}
