package audit

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestTimingStage(t *testing.T) {
	Convey("Given a TimingStage instance", t, func() {
		ctx := context.Background()
		stage := NewTimingStage(ctx)

		feed := func(s *TimingStage, ms ...*data.Measurement) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			_ = s.Run()
			return s.Violations()
		}

		Convey("When auditing well-synchronized monotonic measurements", func() {
			base := time.Now().UnixNano()
			measurements := make([]*data.Measurement, 0, 10)

			for idx := range int64(10) {
				venueTime := time.Unix(0, base+idx*100_000_000)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", venueTime, venueTime)
				measurement.SeqIdx = idx + 1
				measurement.Tick = idx + 1
				measurement.Timestamp = base + idx*100_000_000 + 10_000_000
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing sequence inversions where venue time moves backwards", func() {
			base := time.Now().UnixNano()
			measurements := make([]*data.Measurement, 0, 3)

			firstTime := time.Unix(0, base+200_000_000)
			first := data.NewMeasurement(1, "BTC/USD", "test", firstTime, firstTime)
			first.SeqIdx = 1
			first.Timestamp = base + 210_000_000
			measurements = append(measurements, first)

			invertedTime := time.Unix(0, base+100_000_000)
			inverted := data.NewMeasurement(1, "BTC/USD", "test", invertedTime, invertedTime)
			inverted.SeqIdx = 2
			inverted.Timestamp = base + 220_000_000
			measurements = append(measurements, inverted)

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasInversion := false
			for _, violation := range violations {
				if violation.Name == "timing.sequence_inversion" {
					hasInversion = true
				}
			}
			So(hasInversion, ShouldBeTrue)
		})

		Convey("When auditing sequence index non-increasing violations", func() {
			base := time.Now().UnixNano()
			measurements := make([]*data.Measurement, 0, 2)

			firstTime := time.Unix(0, base+100_000_000)
			first := data.NewMeasurement(1, "BTC/USD", "test", firstTime, firstTime)
			first.SeqIdx = 5
			first.Timestamp = base + 110_000_000
			measurements = append(measurements, first)

			secondTime := time.Unix(0, base+200_000_000)
			second := data.NewMeasurement(1, "BTC/USD", "test", secondTime, secondTime)
			second.SeqIdx = 5
			second.Timestamp = base + 210_000_000
			measurements = append(measurements, second)

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasSeqInversion := false
			for _, violation := range violations {
				if violation.Name == "timing.seq_idx_inversion" {
					hasSeqInversion = true
				}
			}
			So(hasSeqInversion, ShouldBeTrue)
		})

		Convey("When auditing latency jitter spikes", func() {
			base := time.Now().UnixNano()
			measurements := make([]*data.Measurement, 0, 5)

			for idx := range int64(4) {
				venueTime := time.Unix(0, base+idx*100_000_000)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", venueTime, venueTime)
				measurement.SeqIdx = idx + 1
				measurement.Timestamp = base + idx*100_000_000 + 10_000_000
				measurements = append(measurements, measurement)
			}

			spikeTime := time.Unix(0, base+400_000_000)
			spike := data.NewMeasurement(1, "BTC/USD", "test", spikeTime, spikeTime)
			spike.SeqIdx = 5
			spike.Timestamp = base + 400_000_000 + 800_000_000
			measurements = append(measurements, spike)

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasSpike := false
			for _, violation := range violations {
				if violation.Name == "timing.latency_spike" {
					hasSpike = true
				}
			}
			So(hasSpike, ShouldBeTrue)
		})

		Convey("When auditing negative latency clock skew", func() {
			base := time.Now().UnixNano()
			venueTime := time.Unix(0, base+100_000_000)
			skewed := data.NewMeasurement(1, "BTC/USD", "test", venueTime, venueTime)
			skewed.SeqIdx = 1
			skewed.Timestamp = base + 50_000_000

			violations := feed(stage, skewed)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasSkew := false
			for _, violation := range violations {
				if violation.Name == "timing.negative_drift" {
					hasSkew = true
				}
			}
			So(hasSkew, ShouldBeTrue)
		})

		Convey("When auditing zero timestamps and nil measurements", func() {
			nilViolations := feed(stage, nil)
			So(len(nilViolations), ShouldEqual, 1)
			So(nilViolations[0].Name, ShouldEqual, "measurement")

			zeroVenue := data.NewMeasurement(1, "BTC/USD", "test", time.Time{}, time.Time{})
			zeroVenue.SeqIdx = 1
			zeroVenue.Timestamp = 123456
			zeroStage := NewTimingStage(ctx)
			zeroViolations := feed(zeroStage, zeroVenue)
			So(len(zeroViolations), ShouldBeGreaterThanOrEqualTo, 1)
		})
	})
}
