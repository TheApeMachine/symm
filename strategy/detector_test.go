package strategy

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestDetector(t *testing.T) {
	Convey("Detector WORM tape scanner", t, func() {
		detector := NewDetector()

		Convey("Initial state has empty queue", func() {
			_, ok := detector.Next()
			So(ok, ShouldBeFalse)
		})

		Convey("Detects upward excursion and enqueues aligned trajectory", func() {
			now := time.Now()
			measurements := make([]*data.Measurement[float64], 0, 50)

			prices := []float64{
				100, 100, 100, 99, 98, 97, 96, 95.5, 95, 95,
				97, 100, 105, 110, 115, 118, 120,
				117, 115, 114,
			}

			for index, priceValue := range prices {
				seqIdx := int64(index)
				measurement := data.NewMeasurement[float64]("spot:ticker", nil)
				measurement.Label = "XXBTZUSD"
				measurement.SeqIdx = seqIdx
				measurement.At = now.Add(time.Duration(index) * time.Second)
				spread := 1.0
				midpoint := priceValue
				measurement.SetMetric("price", data.NewMetric[float64]("price", data.UnitCurrency, data.TimescaleInstantaneous, midpoint, spread).Write(priceValue))
				measurement.SetMetric("bid", data.Metric[float64]{Raw: priceValue - 0.5, Exact: decimal.NewFromFloat64(priceValue - 0.5)})
				measurement.SetMetric("ask", data.Metric[float64]{Raw: priceValue + 0.5, Exact: decimal.NewFromFloat64(priceValue + 0.5)})
				measurements = append(measurements, measurement)

				// Interleave a signal measurement at the same sequence
				signal := data.NewMeasurement[float64]("cvd", nil)
				signal.Label = "XXBTZUSD"
				signal.SeqIdx = seqIdx
				signal.At = now.Add(time.Duration(index) * time.Second)
				signal.SetMetric("cumulative_volume_delta", data.NewMetric[float64]("cumulative_volume_delta", data.UnitVolume, data.TimescaleRollingWindow, 0, 100.0).Write(float64(index)*10.0))
				measurements = append(measurements, signal)
			}

			detector.Scan(measurements)

			trajectory, ok := detector.Next()
			So(ok, ShouldBeTrue)
			So(trajectory.Symbol, ShouldEqual, "XXBTZUSD")
			So(trajectory.EntryAsk, ShouldNotBeNil)
			So(trajectory.ExitBid, ShouldNotBeNil)
			So(len(trajectory.Precursor), ShouldBeGreaterThan, 0)
			So(len(trajectory.Holding), ShouldBeGreaterThan, 0)
			So(len(trajectory.Ticks[0]), ShouldBeGreaterThan, 0)
			So(len(trajectory.Ticks[1]), ShouldBeGreaterThan, 0)

			hasCvdPrecursor := false
			for _, precursorMeas := range trajectory.Precursor {
				if precursorMeas.Source == "cvd" {
					hasCvdPrecursor = true
					break
				}
			}
			So(hasCvdPrecursor, ShouldBeTrue)

			So(trajectory.Ticks[0][0].Label, ShouldEqual, "XXBTZUSD")
			So(trajectory.Ticks[1][0].Label, ShouldEqual, "XXBTZUSD")
		})
	})
}
