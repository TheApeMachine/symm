package correlation

import (
	"math"
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
			So(r2.Error(), ShouldBeNil)

			metrics2 := make([]float64, 0)
			for m := range r2.Read() {
				metrics2 = append(metrics2, m.Metric.Raw)
			}
			So(len(metrics2), ShouldEqual, len(outputKeys)+1)
		})

		Convey("When repeated observations arrive with zero return energy, no NaN metrics surface", func() {
			mBTC2 := data.NewMeasurement(
				now.Add(2*time.Second).UnixNano(),
				"BTC/USD",
				"spot:trade",
				system.SeqIdx.Add(1),
				system.Tick.Add(1),
			)
			mBTC2.At = now.Add(2 * time.Second)
			mBTC2.From = now.Add(2 * time.Second)
			mBTC2.Write(
				data.NewMetric("price", 50000, data.UnitPrice, data.TimescaleInstantaneous),
			)

			rBTC2 := signal.Step(mBTC2)
			So(rBTC2, ShouldNotBeNil)
			So(rBTC2.Error(), ShouldBeNil)

			for m := range rBTC2.Read() {
				So(math.IsNaN(m.Metric.Raw), ShouldBeFalse)
			}
		})

		Convey("When prices move and return energy is valid, correlation is defined", func() {
			mETH2 := data.NewMeasurement(
				now.Add(2*time.Second).UnixNano(),
				"ETH/USD",
				"spot:trade",
				system.SeqIdx.Add(1),
				system.Tick.Add(1),
			)
			mETH2.At = now.Add(2 * time.Second)
			mETH2.From = now.Add(2 * time.Second)
			mETH2.Write(
				data.NewMetric("price", 3100, data.UnitPrice, data.TimescaleInstantaneous),
			)

			rETH2 := signal.Step(mETH2)
			So(rETH2, ShouldNotBeNil)
			So(rETH2.Error(), ShouldBeNil)

			mBTC3 := data.NewMeasurement(
				now.Add(3*time.Second).UnixNano(),
				"BTC/USD",
				"spot:trade",
				system.SeqIdx.Add(1),
				system.Tick.Add(1),
			)
			mBTC3.At = now.Add(3 * time.Second)
			mBTC3.From = now.Add(3 * time.Second)
			mBTC3.Write(
				data.NewMetric("price", 51000, data.UnitPrice, data.TimescaleInstantaneous),
			)

			rBTC3 := signal.Step(mBTC3)
			So(rBTC3, ShouldNotBeNil)
			So(rBTC3.Error(), ShouldBeNil)

			for m := range rBTC3.Read() {
				So(math.IsNaN(m.Metric.Raw), ShouldBeFalse)
			}
		})
	})
}
