package audit

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

func newTestMeasurement(seqIdx, tick, atNs, timestampNs int64) *data.Value {
	val := data.NewValue("test", "tick", "BTC/USD")
	val.SeqIdx = uint64(seqIdx)
	val.Tick = uint64(tick)
	val.Timestamp = uint64(timestampNs)
	if atNs > 0 {
		data.AppendVector(val, []float64{float64(atNs)}, "at")
	}
	return val
}

func TestTimingStage(t *testing.T) {
	Convey("Given a TimingStage instance", t, func() {
		ctx := context.Background()
		stage := NewTimingStage(ctx)

		feed := func(s *TimingStage, ms ...*data.Value) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		Convey("When auditing well-synchronized monotonic measurements", func() {
			base := time.Now().UnixNano()
			measurements := make([]*data.Value, 0, 10)

			for idx := range int64(10) {
				m := newTestMeasurement(
					idx+1,
					idx+1,
					base+idx*100_000_000,
					base+idx*100_000_000+10_000_000,
				)
				measurements = append(measurements, m)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing sequence inversions where venue time moves backwards", func() {
			base := time.Now().UnixNano()
			measurements := make([]*data.Value, 0, 3)

			first := newTestMeasurement(1, 1, base+200_000_000, base+210_000_000)
			measurements = append(measurements, first)

			inverted := newTestMeasurement(2, 2, base+100_000_000, base+220_000_000)
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
			measurements := make([]*data.Value, 0, 2)

			first := newTestMeasurement(5, 1, base+100_000_000, base+110_000_000)
			measurements = append(measurements, first)

			second := newTestMeasurement(5, 2, base+200_000_000, base+210_000_000)
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
			measurements := make([]*data.Value, 0, 5)

			for idx := range int64(4) {
				m := newTestMeasurement(
					idx+1,
					idx+1,
					base+idx*100_000_000,
					base+idx*100_000_000+10_000_000,
				)
				measurements = append(measurements, m)
			}

			spike := newTestMeasurement(5, 5, base+400_000_000, base+400_000_000+800_000_000)
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
			skewed := newTestMeasurement(1, 1, base+100_000_000, base+50_000_000)

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

			zeroVenue := newTestMeasurement(1, 1, 0, 123456)
			zeroStage := NewTimingStage(ctx)
			zeroViolations := feed(zeroStage, zeroVenue)
			So(len(zeroViolations), ShouldBeGreaterThanOrEqualTo, 1)
		})
	})
}
