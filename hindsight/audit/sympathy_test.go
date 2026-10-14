package audit

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestSympathyStage(t *testing.T) {
	Convey("Given a SympathyStage instance", t, func() {
		ctx := context.Background()
		stage := NewSympathyStage(ctx)

		feed := func(s *SympathyStage, ms ...*data.Measurement) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			_ = s.Run()
			return s.Violations()
		}

		Convey("When auditing independent asynchronous streams", func() {
			base := time.Now().UnixNano()
			measurements := make([]*data.Measurement, 0, 40)

			// Source A updates at intervals +10ms, +20ms, etc.
			for idx := range int64(20) {
				venueTime := time.Unix(0, base+idx*20_000_000)
				measurement := data.NewMeasurement(1, "BTC/USD", "source_a", venueTime, venueTime)
				val := 100.0 + float64(idx)*0.5
				measurement.Write(data.NewMetric("price", val, data.UnitPrice, data.TimescaleInstantaneous))
				measurements = append(measurements, measurement)
			}

			// Source B updates at offset intervals +5ms, +25ms, etc. with sinusoidal values (uncorrelated)
			for idx := range int64(20) {
				venueTime := time.Unix(0, base+idx*20_000_000+5_000_000)
				measurement := data.NewMeasurement(1, "BTC/USD", "source_b", venueTime, venueTime)
				val := 100.0 + 5.0*math.Sin(float64(idx)*math.Pi/2.0)
				measurement.Write(data.NewMetric("signal", val, data.UnitDimensionless, data.TimescaleInstantaneous))
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing cross-source clone streams with continuous-time Hayashi-Yoshida overlap", func() {
			base := time.Now().UnixNano()
			measurements := make([]*data.Measurement, 0, 40)

			// Source A updates at intervals +100ms
			for idx := range int64(20) {
				venueTime := time.Unix(0, base+idx*100_000_000)
				measurement := data.NewMeasurement(1, "BTC/USD", "producer_one", venueTime, venueTime)
				val := 100.0 * math.Exp(float64(idx+1)*0.01)
				measurement.Write(data.NewMetric("quote", val, data.UnitPrice, data.TimescaleInstantaneous))
				measurements = append(measurements, measurement)
			}

			// Producer Two is an asynchronous copy with slight timestamp jitter (+10ms) but identical returns
			for idx := range int64(20) {
				venueTime := time.Unix(0, base+idx*100_000_000+10_000_000)
				measurement := data.NewMeasurement(1, "BTC/USD", "producer_two", venueTime, venueTime)
				val := 100.0 * math.Exp(float64(idx+1)*0.01)
				measurement.Write(data.NewMetric("quote", val, data.UnitPrice, data.TimescaleInstantaneous))
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasSympathyViolation := false
			for _, violation := range violations {
				if violation.Name == "sympathy.cross_stream_redundancy" {
					hasSympathyViolation = true
				}
			}
			So(hasSympathyViolation, ShouldBeTrue)
			So(stage.Report(), ShouldContainSubstring, "sympathy.cross_stream_redundancy")
		})

		Convey("When auditing nil measurements and nil metric entries", func() {
			nilViolations := feed(stage, nil)
			So(len(nilViolations), ShouldEqual, 1)
			So(nilViolations[0].Name, ShouldEqual, "measurement")

			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
			measurement.Write(&data.Metric{})
			entryStage := NewSympathyStage(ctx)
			violations := feed(entryStage, measurement)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)
		})
	})
}
