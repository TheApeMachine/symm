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

func TestGaugeReadingMeans(t *testing.T) {
	Convey("Given readings in the ring", t, func() {
		gauge := &Gauge{readings: ring.New(8)}
		gauge.readings.Value = &Reading{Confidence: 0.8, Surprise: 2.0}
		gauge.readings = gauge.readings.Next()
		gauge.readings.Value = &Reading{Confidence: 0.4, Surprise: 1.0}
		gauge.readings = gauge.readings.Next()

		meanConfidence, meanSurprise := gauge.readingMeans()

		Convey("It should average positive confidence and surprise", func() {
			So(meanConfidence, ShouldAlmostEqual, 0.6, 1e-9)
			So(meanSurprise, ShouldAlmostEqual, 1.5, 1e-9)
		})
	})

	Convey("Given no positive readings", t, func() {
		gauge := &Gauge{readings: ring.New(8)}

		meanConfidence, meanSurprise := gauge.readingMeans()

		Convey("It should report zero means", func() {
			So(meanConfidence, ShouldEqual, 0)
			So(meanSurprise, ShouldEqual, 0)
		})
	})
}

func TestGaugeSurpriseThreshold(t *testing.T) {
	Convey("Given per-signal threshold config", t, func() {
		viper.Set("signals.fluid.surprise_threshold", 2.5)
		viper.Set("signals.sentiment.surge_threshold", 3.0)
		viper.Set("signals.exhaust.surprise_threshold", 1.5)

		gauge := &Gauge{source: string(logic.SourceFluid)}
		sentimentGauge := &Gauge{source: string(logic.SourceSentiment)}
		exhaustGauge := &Gauge{source: string(logic.SourceExhaustion)}

		Convey("It should read the matching config key", func() {
			So(gauge.surpriseThreshold(), ShouldEqual, 2.5)
			So(sentimentGauge.surpriseThreshold(), ShouldEqual, 3.0)
			So(exhaustGauge.surpriseThreshold(), ShouldEqual, 1.5)
		})
	})

	Convey("Given out-of-range threshold config", t, func() {
		viper.Set("signals.cvd.surprise_threshold", 9.0)

		gauge := &Gauge{source: string(logic.SourceCVD)}

		Convey("It should clamp to the supported range", func() {
			So(gauge.surpriseThreshold(), ShouldEqual, 5.0)
		})
	})
}

func TestGaugeWarmupState(t *testing.T) {
	Convey("Given no registered symbols yet", t, func() {
		gauge := &Gauge{
			warmupCapacity:  64,
			expectedSymbols: make(map[string]struct{}),
		}

		samples, minSamples, calibrating, calibrated := gauge.warmupState()

		Convey("It should report zero progress", func() {
			So(samples, ShouldEqual, 0)
			So(minSamples, ShouldEqual, 64)
			So(calibrating, ShouldBeTrue)
			So(calibrated, ShouldBeFalse)
		})
	})

	Convey("Given registered symbols and ring writes", t, func() {
		gauge := &Gauge{
			warmupCapacity:   4,
			warmupSamples:    6,
			minWarmupSamples: 8,
			expectedSymbols: map[string]struct{}{
				"BTC/EUR": {},
				"ETH/EUR": {},
			},
		}

		samples, minSamples, calibrating, calibrated := gauge.warmupState()

		Convey("It should use the running warmup sample total", func() {
			So(samples, ShouldEqual, 6)
			So(minSamples, ShouldEqual, 8)
			So(calibrating, ShouldBeTrue)
			So(calibrated, ShouldBeFalse)
		})
	})

	Convey("Given every registered symbol is warmed up", t, func() {
		gauge := &Gauge{
			warmupCapacity:   4,
			warmupSamples:    4,
			minWarmupSamples: 4,
			expectedSymbols: map[string]struct{}{
				"BTC/EUR": {},
			},
		}

		_, _, calibrating, calibrated := gauge.warmupState()

		Convey("It should mark warmup complete", func() {
			So(calibrating, ShouldBeFalse)
			So(calibrated, ShouldBeTrue)
		})
	})

	Convey("Given registered symbols before any warmed writes", t, func() {
		gauge := &Gauge{
			warmupCapacity:  16,
			expectedSymbols: map[string]struct{}{"BTC/EUR": {}},
		}

		samples, minSamples, calibrating, calibrated := gauge.warmupState()

		Convey("It should stay calibrating until warmup targets exist", func() {
			So(samples, ShouldEqual, 0)
			So(minSamples, ShouldEqual, 16)
			So(calibrating, ShouldBeTrue)
			So(calibrated, ShouldBeFalse)
		})
	})
}

func TestGaugeRegisterSymbols(t *testing.T) {
	Convey("Given a symbol batch announcement", t, func() {
		gauge := &Gauge{
			warmupCapacity:  64,
			expectedSymbols: make(map[string]struct{}),
			warmupSymbols:   make(map[string]struct{}),
		}

		gauge.RegisterSymbols([]string{"BTC/USD", "ETH/USD"})

		Convey("It should track symbols without inflating the warmup denominator", func() {
			So(len(gauge.expectedSymbols), ShouldEqual, 2)
			So(gauge.minWarmupSamples, ShouldEqual, 0)
		})

		Convey("It should still count warmup after a universe announcement", func() {
			gauge.RecordWarmup("BTC/USD", true)

			So(gauge.warmupSamples, ShouldEqual, 1)
			So(gauge.minWarmupSamples, ShouldEqual, 64)
		})
	})
}

func TestGaugeRecordWarmup(t *testing.T) {
	Convey("Given a warmed write for a new symbol", t, func() {
		gauge := &Gauge{
			warmupCapacity:  4,
			expectedSymbols: make(map[string]struct{}),
			warmupSymbols:   make(map[string]struct{}),
		}

		gauge.RecordWarmup("BTC/EUR", true)
		gauge.RecordWarmup("BTC/EUR", true)

		Convey("It should register the symbol and count warmed writes", func() {
			So(gauge.warmupSamples, ShouldEqual, 2)
			So(gauge.minWarmupSamples, ShouldEqual, 4)
		})
	})

	Convey("Given a measure failure after Record", t, func() {
		gauge := &Gauge{
			warmupCapacity:  4,
			expectedSymbols: make(map[string]struct{}),
			warmupSymbols:   make(map[string]struct{}),
		}

		gauge.RecordWarmup("ETH/EUR", true)

		Convey("It should still advance warmup before any gauge publish", func() {
			So(gauge.warmupSamples, ShouldEqual, 1)
			So(gauge.minWarmupSamples, ShouldEqual, 4)
		})
	})
}

func TestNewGauge(t *testing.T) {
	Convey("Given telemetry capacity config", t, func() {
		viper.Set("telemetry.gauge.readings_capacity", 32)
		viper.Set("signals.fluid.measurements_capacity", 16)
		viper.Set("signals.exhaust.measurements_capacity", 48)
		viper.Set("signals.pumpdump.measurements_capacity", 64)

		fluidGauge, fluidErr := NewGauge(nil, logic.SourceFluid)
		exhaustGauge, exhaustErr := NewGauge(nil, logic.SourceExhaustion)
		pumpdumpGauge, pumpdumpErr := NewGauge(nil, logic.SourcePumpDump)

		Convey("It should allocate the reading ring and warmup capacity", func() {
			So(fluidErr, ShouldBeNil)
			So(fluidGauge.readings.Len(), ShouldEqual, 32)
			So(fluidGauge.warmupCapacity, ShouldEqual, 16)

			So(exhaustErr, ShouldBeNil)
			So(exhaustGauge.warmupCapacity, ShouldEqual, 48)

			So(pumpdumpErr, ShouldBeNil)
			So(pumpdumpGauge.warmupCapacity, ShouldEqual, 64)
		})
	})

	Convey("Given missing measurements capacity", t, func() {
		viper.Set("signals.cvd.measurements_capacity", 0)

		gauge, err := NewGauge(nil, logic.SourceCVD)

		Convey("It should return an error", func() {
			So(gauge, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestGaugePublishThrottled(t *testing.T) {
	Convey("Given a recent ui publish", t, func() {
		viper.Set("telemetry.gauge.publish_interval", 100*time.Millisecond)

		gauge := &Gauge{
			readings:      ring.New(8),
			lastPublishAt: time.Now(),
		}

		Convey("It should suppress back-to-back ui frames", func() {
			So(gauge.publishThrottled(), ShouldBeTrue)
		})
	})
}

func TestGaugePublishWarmup(t *testing.T) {
	Convey("Given warmup progress without publishable readings", t, func() {
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

		gauge, gaugeErr := NewGauge(bus, logic.SourceFluid)

		So(gaugeErr, ShouldBeNil)

		gauge.RecordWarmup("BTC/EUR", true)
		gauge.RecordWarmup("BTC/EUR", true)

		err := gauge.PublishWarmup()

		Convey("It should publish calibrating gauge frames", func() {
			So(err, ShouldBeNil)
		})
	})

	Convey("Given a calibrated gauge", t, func() {
		viper.Set("telemetry.gauge.publish_interval", 0)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		bus := internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelUI},
			nil,
		)

		gauge, gaugeErr := NewGauge(bus, logic.SourceFluid)

		So(gaugeErr, ShouldBeNil)

		gauge.expectedSymbols["BTC/EUR"] = struct{}{}
		gauge.warmupSymbols["BTC/EUR"] = struct{}{}
		gauge.minWarmupSamples = 4
		gauge.warmupSamples = 4

		err := gauge.PublishWarmup()

		Convey("It should skip zero-confidence warmup publishes", func() {
			So(err, ShouldBeNil)
		})
	})
}

func BenchmarkGaugeReadingMeans(b *testing.B) {
	gauge := &Gauge{readings: ring.New(256)}

	for index := range 64 {
		gauge.readings.Value = &Reading{
			Confidence: 0.5,
			Surprise:   1.2 + float64(index)*0.01,
		}
		gauge.readings = gauge.readings.Next()
	}

	b.ReportAllocs()

	for b.Loop() {
		meanConfidence, meanSurprise := gauge.readingMeans()
		_ = meanConfidence
		_ = meanSurprise
	}
}
