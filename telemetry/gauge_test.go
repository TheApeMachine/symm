package telemetry

import (
	"container/ring"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
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
}

func TestGaugeRecordWarmup(t *testing.T) {
	Convey("Given a registered symbol", t, func() {
		gauge := &Gauge{
			warmupCapacity:   4,
			expectedSymbols:  map[string]struct{}{"BTC/EUR": {}},
			minWarmupSamples: 4,
		}

		gauge.recordWarmup("BTC/EUR", true)
		gauge.recordWarmup("BTC/EUR", true)

		Convey("It should increment warmup on each warmed write", func() {
			So(gauge.warmupSamples, ShouldEqual, 2)
		})
	})
}

func TestNewGauge(t *testing.T) {
	Convey("Given telemetry capacity config", t, func() {
		viper.Set("telemetry.gauge.readings_capacity", 32)
		viper.Set("signals.fluid.measurements_capacity", 16)
		viper.Set("signals.exhaust.measurements_capacity", 48)

		fluidGauge, fluidErr := NewGauge(nil, logic.SourceFluid)
		exhaustGauge, exhaustErr := NewGauge(nil, logic.SourceExhaustion)

		Convey("It should allocate the reading ring and warmup capacity", func() {
			So(fluidErr, ShouldBeNil)
			So(fluidGauge.readings.Len(), ShouldEqual, 32)
			So(fluidGauge.warmupCapacity, ShouldEqual, 16)

			So(exhaustErr, ShouldBeNil)
			So(exhaustGauge.warmupCapacity, ShouldEqual, 48)
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
