package resonance

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestAlphaControllerDoesNotRatchet pins the defect that made the previous
controller walk its pace to a bound and stay there. A stationary error stream
carries no evidence that the pace should change, so the pace must not drift.
*/
func TestAlphaControllerDoesNotRatchet(t *testing.T) {
	Convey("Given a stationary reconstruction error stream", t, func() {
		controller := NewAlphaController(restAlpha, minAlpha, maxAlpha)
		alpha := restAlpha

		/*
			A constant error would leave the z-score's span at zero and never
			report ready, which is a degenerate stream rather than a stationary
			one. A deterministic oscillation has a stable mean and dispersion,
			so it is stationary while still exercising the ready path.
		*/
		early := 0.0

		for tick := range 20_000 {
			alpha = controller.Update(1.0+0.01*math.Sin(float64(tick)), 0.5)

			if tick == 2_000 {
				early = alpha
			}
		}

		/*
			The pace is expected to move: it tracks local evidence, and a
			stationary stream still produces individual readings above and below
			the band. What must not happen is accumulation, so the bound is on
			how far the pace can stray from rest rather than on it holding rest
			exactly.

			Running ten times longer than the drift check is the point. A
			ratchet compounds with tick count, so a formulation that survives
			two thousand ticks and fails at twenty thousand is still a ratchet.
		*/
		Convey("Then the pace oscillates about rest rather than walking to a bound", func() {
			So(alpha, ShouldBeBetween, restAlpha/2, restAlpha*2)
			So(alpha, ShouldBeLessThan, maxAlpha)
			So(alpha, ShouldBeGreaterThan, minAlpha)
		})

		Convey("Then ten times more ticks does not compound the excursion", func() {
			So(math.Abs(alpha-restAlpha), ShouldBeLessThan, restAlpha)
			So(math.Abs(early-restAlpha), ShouldBeLessThan, restAlpha)
		})
	})

	Convey("Given an error stream with intermittent spikes", t, func() {
		controller := NewAlphaController(restAlpha, minAlpha, maxAlpha)
		alpha := restAlpha

		/*
			This is the shape that pinned the old controller at its ceiling: a
			spike every seventh tick, checked before any damping branch could
			run, with no restoring term to bring the pace back down.
		*/
		for tick := range 20_000 {
			reading := 1.0 + 0.01*math.Sin(float64(tick))

			if tick%7 == 0 {
				reading = 5.0
			}

			alpha = controller.Update(reading, 0.5)
		}

		/*
			This is precisely the stream that pinned the previous controller at
			its ceiling within about forty ticks and held it there indefinitely.
		*/
		Convey("Then the pace stays near rest instead of pinning at the ceiling", func() {
			So(alpha, ShouldBeLessThan, maxAlpha)
			So(alpha, ShouldBeGreaterThan, minAlpha)
			So(alpha, ShouldBeLessThan, restAlpha*2)
		})
	})
}

/*
TestAlphaControllerRespondsToRegimeShift pins the behaviour the controller
exists for: a sustained rise in error means the retained model no longer
describes the market, and the pace must rise so it is replaced faster.
*/
func TestAlphaControllerRespondsToRegimeShift(t *testing.T) {
	Convey("Given a quiet stretch followed by a sustained error rise", t, func() {
		controller := NewAlphaController(restAlpha, minAlpha, maxAlpha)

		for tick := range 300 {
			controller.Update(1.0+0.01*math.Sin(float64(tick)), 0.5)
		}

		quiet := controller.Update(1.0, 0.5)

		for range 300 {
			controller.Update(50.0, 0.5)
		}

		shifted := controller.Update(50.0, 0.5)

		Convey("Then the pace rises off the quiet level and stays bounded", func() {
			So(quiet, ShouldBeBetween, restAlpha/2, restAlpha*2)
			So(shifted, ShouldBeGreaterThan, quiet*2)
			So(shifted, ShouldBeLessThanOrEqualTo, maxAlpha)
		})
	})
}

/*
TestAlphaControllerWarmupHoldsPace pins that an unready calibration is a hold
rather than a move. Warmup follows every schema change, so acting on it would
move the pace on no evidence each time the schema settled.
*/
func TestAlphaControllerWarmupHoldsPace(t *testing.T) {
	Convey("Given a controller that has seen a single reading", t, func() {
		controller := NewAlphaController(restAlpha, minAlpha, maxAlpha)
		alpha := controller.Update(7.0, 3.0)

		Convey("Then the pace is unchanged", func() {
			So(alpha, ShouldEqual, restAlpha)
		})
	})
}

/*
TestErrorCalibratorIsUniform pins that confidence is a calibrated probability.
Readings drawn from a stable distribution must produce quantiles spread across
the unit interval, whatever the scale of those readings, because downstream
consumers read confidence as a probability and clamp it to [0,1].
*/
func TestErrorCalibratorIsUniform(t *testing.T) {
	Convey("Given readings on a scale where exp(-x) would saturate", t, func() {
		calibrator := newErrorCalibrator()

		/*
			A live schema of a few hundred features puts the error norm well
			above the range exp(-x) resolves: exp(-40) is zero to any consumer.
			The calibrator must still spread these across the interval.
		*/
		for tick := range errorCalibratorWindow {
			calibrator.Quantile(40.0 + 5.0*math.Sin(float64(tick)))
		}

		low := calibrator.Quantile(35.1)
		high := calibrator.Quantile(44.9)
		middle := calibrator.Quantile(40.0)

		Convey("Then quantiles span the interval instead of collapsing to zero", func() {
			So(low, ShouldBeGreaterThan, 0.8)
			So(high, ShouldBeLessThan, 0.2)
			So(middle, ShouldBeBetween, 0.2, 0.8)

			So(math.Exp(-40.0), ShouldAlmostEqual, 0, 1e-12)
		})
	})

	Convey("Given a calibrator with no history", t, func() {
		calibrator := newErrorCalibrator()

		Convey("Then the first reading claims no confidence", func() {
			So(calibrator.Quantile(1.0), ShouldEqual, 0)
		})
	})

	Convey("Given more readings than the retained window holds", t, func() {
		calibrator := newErrorCalibrator()

		for range errorCalibratorWindow * 2 {
			calibrator.Quantile(100.0)
		}

		Convey("Then a regime of small errors scores confidently", func() {
			So(calibrator.Quantile(1.0), ShouldEqual, 1)
		})
	})
}
