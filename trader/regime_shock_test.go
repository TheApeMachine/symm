package trader

import (
	"testing"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRegimeShockBreakerObserve(t *testing.T) {
	Convey("Given a warmed correlation shock detector", t, func() {
		original := *config.System
		config.System.RegimeShockWindow = 8
		config.System.RegimeShockMinSamples = 6
		config.System.RegimeShockZScore = 4
		config.System.RegimeShockRecoverySamples = 3
		t.Cleanup(func() { *config.System = original })

		breaker := newRegimeShockBreaker()

		for range 6 {
			breaker.Observe(engine.Measurement{Source: "correlation", Confidence: 0.2})
		}

		breaker.Observe(engine.Measurement{Source: "correlation", Confidence: 0.95})

		Convey("It should activate on a dynamic upper-bound breach", func() {
			So(breaker.Active(), ShouldBeTrue)
			So(breaker.Mutes("hawkes"), ShouldBeTrue)
			So(breaker.Mutes("cvd"), ShouldBeFalse)
		})

		Convey("It should recover after fresh non-shock samples", func() {
			for range 3 {
				breaker.Observe(engine.Measurement{Source: "correlation", Confidence: 0.21})
			}

			So(breaker.Active(), ShouldBeFalse)
		})
	})
}

func TestSourceCalibratorCalibrateConfidence(t *testing.T) {
	Convey("Given an active regime shock", t, func() {
		original := *config.System
		config.System.RegimeShockWindow = 8
		config.System.RegimeShockMinSamples = 6
		config.System.RegimeShockZScore = 4
		config.System.RegimeShockTrustFloor = 0.01
		t.Cleanup(func() { *config.System = original })

		breaker := newRegimeShockBreaker()
		calibrator := newSourceCalibrator(breaker)

		for range 6 {
			breaker.Observe(engine.Measurement{Source: "fluid", Confidence: 0.2})
		}

		breaker.Observe(engine.Measurement{Source: "fluid", Confidence: 0.95})

		Convey("It should mute parameterized sources and preserve robust flow", func() {
			So(calibrator.CalibrateConfidence("hawkes", 0.8), ShouldAlmostEqual, 0.008, 1e-12)
			So(calibrator.CalibrateConfidence("cvd", 0.8), ShouldEqual, 0.8)
		})
	})
}

func BenchmarkRegimeShockBreakerObserve(b *testing.B) {
	original := *config.System
	config.System.RegimeShockWindow = 128
	config.System.RegimeShockMinSamples = 64
	b.Cleanup(func() { *config.System = original })

	breaker := newRegimeShockBreaker()
	measurement := engine.Measurement{Source: "correlation", Confidence: 0.2}

	for range 64 {
		breaker.Observe(measurement)
	}

	b.ReportAllocs()

	for b.Loop() {
		breaker.Observe(measurement)
	}
}
