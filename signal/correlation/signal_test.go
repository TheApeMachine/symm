package correlation

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

func TestCorrelationSignalMetrics(t *testing.T) {
	Convey("Given a READY correlation signal", t, func() {
		now := time.Now()
		measurement := data.NewMeasurement(
			now.UnixNano(),
			"BTC/USD",
			"spot:trade",
			system.SeqIdx.Add(1),
			system.Tick.Add(1),
		)
		measurement.At = now
		measurement.From = now
		measurement.Write(
			data.NewMetric("price", 50000, data.UnitPrice, data.TimescaleInstantaneous),
		)

		signal := NewSignal(t.Context())
		signal.Transition(runtime.READY)
		result := signal.Step(measurement)

		Convey("Then the result should have the expected metrics", func() {
			So(result, ShouldNotBeNil)

			metrics := make([]float64, 0)

			for m := range result.Read() {
				metrics = append(metrics, m.Metric.Raw)
			}

			// We expect the 34 output keys + 1 original metric (price) from prior measurement
			So(len(metrics), ShouldEqual, len(outputKeys)+1)
		})

		Convey("When a second peer symbol arrives, it correlates against the first", func() {
			m2 := data.NewMeasurement(
				now.Add(time.Second).UnixNano(),
				"ETH/USD",
				"spot:trade",
				system.SeqIdx.Add(1),
				system.Tick.Add(1),
			)
			m2.At = now.Add(time.Second)
			m2.From = now.Add(time.Second)
			m2.Write(
				data.NewMetric("price", 3000, data.UnitPrice, data.TimescaleInstantaneous),
			)

			r2 := signal.Step(m2)
			So(r2, ShouldNotBeNil)

			metrics2 := make([]float64, 0)
			for m := range r2.Read() {
				metrics2 = append(metrics2, m.Metric.Raw)
			}
			So(len(metrics2), ShouldEqual, len(outputKeys)+1)
		})
	})
}
