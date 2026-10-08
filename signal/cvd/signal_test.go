package cvd

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

func TestCVDSignalMetrics(t *testing.T) {
	Convey("Given a READY CVD signal", t, func() {
		now := time.Now()
		measurement := data.NewMeasurement(
			now.UnixNano(),
			"BTC/USD",
			"spot:trade",
			system.SeqIdx.Add(1),
			system.Tick.Add(1),
			&data.StringEntry{
				Key:   "side",
				Value: "buy",
			},
		)
		measurement.At = now
		measurement.From = now
		measurement.Write(
			data.NewMetric("price", 100, data.UnitPrice, data.TimescaleInstantaneous),
			data.NewMetric("qty", 10, data.UnitQuantity, data.TimescaleInstantaneous),
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

			// We expect the 42 output keys + the 2 original metrics (price, qty) from the prior measurement
			So(len(metrics), ShouldEqual, len(outputKeys)+2)
		})
	})
}
