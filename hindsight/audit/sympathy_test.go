package audit

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

func TestSympathyStage(t *testing.T) {
	Convey("Given a SympathyStage instance", t, func() {
		ctx := context.Background()
		stage := NewSympathyStage(ctx)

		feed := func(s *SympathyStage, ms ...*data.Value) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		Convey("When auditing independent asynchronous streams", func() {
			base := time.Now().UnixNano()
			measurements := make([]*data.Value, 0, 40)

			// Source A updates at intervals +10ms, +20ms, etc.
			for idx := range int64(20) {
				val := data.NewValue("source_a", "signal", "BTC/USD")
				val.Timestamp = uint64(base + idx*20_000_000)
				data.AppendVector(val, []float64{100.0 + float64(idx)*0.5}, "price")
				measurements = append(measurements, val)
			}

			// Source B updates at offset intervals +5ms, +25ms, etc. with sinusoidal values (uncorrelated)
			for idx := range int64(20) {
				val := data.NewValue("source_b", "signal", "BTC/USD")
				val.Timestamp = uint64(base + idx*20_000_000 + 5_000_000)
				data.AppendVector(val, []float64{100.0 + 5.0*math.Sin(float64(idx)*math.Pi/2.0)}, "signal")
				measurements = append(measurements, val)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing cross-source clone streams with continuous-time Hayashi-Yoshida overlap", func() {
			base := time.Now().UnixNano()
			measurements := make([]*data.Value, 0, 40)

			// Source A updates at intervals +100ms
			for idx := range int64(20) {
				val := data.NewValue("producer_one", "signal", "BTC/USD")
				val.Timestamp = uint64(base + idx*100_000_000)
				data.AppendVector(val, []float64{100.0 * math.Exp(float64(idx+1)*0.01)}, "quote")
				measurements = append(measurements, val)
			}

			// Producer Two is an asynchronous copy with slight timestamp jitter (+10ms) but identical returns
			for idx := range int64(20) {
				val := data.NewValue("producer_two", "signal", "BTC/USD")
				val.Timestamp = uint64(base + idx*100_000_000 + 10_000_000)
				data.AppendVector(val, []float64{100.0 * math.Exp(float64(idx+1)*0.01)}, "quote")
				measurements = append(measurements, val)
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

		Convey("When two assets move independently", func() {
			base := time.Now().UnixNano()
			measurements := make([]*data.Value, 0, 40)

			// BTC/USD trending up
			for idx := range int64(20) {
				val := data.NewValue("source_a", "signal", "BTC/USD")
				val.Timestamp = uint64(base + idx*20_000_000)
				data.AppendVector(val, []float64{100.0 + float64(idx)*1.0}, "price")
				measurements = append(measurements, val)
			}

			// ETH/USD independent random oscillation (no continuous-time sympathy)
			for idx := range int64(20) {
				val := data.NewValue("source_b", "signal", "ETH/USD")
				val.Timestamp = uint64(base + idx*20_000_000 + 5_000_000)
				data.AppendVector(val, []float64{100.0 + 10.0*math.Sin(float64(idx)*math.Pi/2.0)}, "price")
				measurements = append(measurements, val)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing nil measurements", func() {
			nilViolations := feed(stage, nil)
			So(len(nilViolations), ShouldEqual, 1)
			So(nilViolations[0].Name, ShouldEqual, "measurement")
		})
	})
}
