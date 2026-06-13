package telemetry

import (
	"container/ring"
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/logic"
)

func TestGaugeReadingSummaryTracksEvidence(t *testing.T) {
	Convey("Given complete readings in the ring", t, func() {
		observedAt := time.Date(2026, 6, 12, 18, 45, 0, 0, time.UTC)
		gauge := &Gauge{readings: ring.New(8)}
		gauge.readings.Value = &Reading{
			Confidence: 0.8,
			Surprise:   2.0,
			Strength:   0.4,
			Elapsed:    10,
			ObservedAt: observedAt,
			Active:     true,
		}
		gauge.readings = gauge.readings.Next()
		gauge.readings.Value = &Reading{
			Confidence: 0.4,
			Surprise:   1.0,
			Strength:   0.2,
			Elapsed:    20,
			ObservedAt: observedAt.Add(time.Second),
			Active:     true,
		}

		summary := gauge.readingSummary()

		Convey("It should summarize health evidence for the ui", func() {
			So(summary.meanConfidence, ShouldAlmostEqual, 0.6, 1e-9)
			So(summary.meanSurprise, ShouldAlmostEqual, 1.5, 1e-9)
			So(summary.meanStrength, ShouldAlmostEqual, 0.3, 1e-9)
			So(summary.meanElapsed, ShouldAlmostEqual, 15, 1e-9)
			So(summary.activeReadings, ShouldEqual, 2)
			So(summary.readingsCapacity, ShouldEqual, 8)
			So(summary.latestObservedAt, ShouldEqual, observedAt.Add(time.Second))
		})
	})
}

func TestGaugePublishIncludesEvidence(t *testing.T) {
	Convey("Given a publishable measurement", t, func() {
		viper.Set("telemetry.gauge.publish_interval", 0)
		viper.Set("telemetry.gauge.readings_capacity", 8)
		viper.Set("signals.fluid.measurements_capacity", 4)
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		bus := internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelUI},
			nil,
		)
		subscriber := internal.NewBus(
			ctx,
			pool,
			nil,
			[]internal.Subscription{internal.Subscribe(internal.ChannelUI, "gauge-evidence-test")},
		)
		gauge, gaugeErr := NewGauge(bus, logic.SourceFluid)

		So(gaugeErr, ShouldBeNil)

		observedAt := time.Date(2026, 6, 12, 18, 45, 0, 0, time.UTC)
		measurement := logic.Measurement{
			Source:     logic.SourceFluid,
			Symbol:     "BTC/EUR",
			Price:      64000,
			Strength:   0.42,
			Volume:     12,
			Spread:     0.2,
			Elapsed:    1,
			Category:   logic.CategoryLaminar,
			Confidence: 0.72,
			Surprise:   1.8,
			ObservedAt: observedAt,
		}

		err := gauge.Publish(measurement)

		Convey("It should publish active evidence for diagnostics", func() {
			So(err, ShouldBeNil)

			frame, receiveErr := subscriber.Receive(internal.ChannelUI)

			So(receiveErr, ShouldBeNil)

			payload, ok := frame.Value.(map[string]any)

			So(ok, ShouldBeTrue)
			So(payload["confidence"], ShouldEqual, 0.72)
			So(payload["surprise"], ShouldEqual, 1.8)
			So(payload["strength"], ShouldEqual, 0.42)
			So(payload["elapsed"], ShouldEqual, float64(1))
			So(payload["active_readings"], ShouldEqual, 1)
			So(payload["readings_capacity"], ShouldEqual, 8)
			So(payload["observed_at"], ShouldEqual, observedAt.Format(time.RFC3339Nano))
		})
	})
}

func BenchmarkGaugeReadingSummary(b *testing.B) {
	observedAt := time.Date(2026, 6, 12, 18, 45, 0, 0, time.UTC)
	gauge := &Gauge{readings: ring.New(256)}

	for index := range 64 {
		gauge.readings.Value = &Reading{
			Confidence: 0.5,
			Surprise:   1.2 + float64(index)*0.01,
			Strength:   0.3 + float64(index)*0.001,
			Elapsed:    10 + float64(index)*0.1,
			ObservedAt: observedAt.Add(time.Duration(index) * time.Second),
			Active:     true,
		}
		gauge.readings = gauge.readings.Next()
	}

	b.ReportAllocs()

	for b.Loop() {
		summary := gauge.readingSummary()
		_ = summary
	}
}
