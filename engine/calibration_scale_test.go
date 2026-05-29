package engine

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCalibrationScaleSeedsToFirstSample(t *testing.T) {
	Convey("Given a fresh scale", t, func() {
		scale := newCalibrationScale(DefaultCalibrationParams().gateParams())

		Convey("It should report a neutral one before any sample", func() {
			So(scale.Scale(), ShouldEqual, 1)
		})

		Convey("The first sample should seed the state", func() {
			scale.Observe(0.8, 0.2)
			So(scale.Scale(), ShouldAlmostEqual, 0.8, 0.0001)
		})
	})
}

func TestCalibrationScaleFastTracksRecovery(t *testing.T) {
	Convey("Given two scales depressed by an identical downside run", t, func() {
		boostedParams := DefaultCalibrationParams().gateParams()
		boostedParams.recoveryFactor = 6
		boostedParams.recoverySamples = 2

		flatParams := boostedParams
		flatParams.recoveryFactor = 1

		boosted := newCalibrationScale(boostedParams)
		flat := newCalibrationScale(flatParams)

		const calmGain = 0.2

		warm := func(scale *calibrationScale) {
			scale.Observe(1, calmGain)
			for i := 0; i < 20; i++ {
				scale.Observe(1, calmGain)
			}
			// Drift the scale below its healthy baseline with modest, in-distribution losses.
			for i := 0; i < 12; i++ {
				scale.Observe(0.6, calmGain)
			}
		}

		warm(boosted)
		warm(flat)

		Convey("Both should be depressed below their baseline", func() {
			So(boosted.Scale(), ShouldBeLessThan, 1)
			So(flat.Scale(), ShouldBeLessThan, 1)
			So(boosted.recoveryOpen(), ShouldBeTrue)
			So(flat.recoveryOpen(), ShouldBeTrue)

			Convey("The same upside sample should lift the boosted scale further", func() {
				beforeBoosted := boosted.Scale()
				beforeFlat := flat.Scale()

				So(beforeBoosted, ShouldAlmostEqual, beforeFlat, 0.05)

				boosted.Observe(1.0, calmGain)
				flat.Observe(1.0, calmGain)

				boostedGain := boosted.Scale() - beforeBoosted
				flatGain := flat.Scale() - beforeFlat

				So(boostedGain, ShouldBeGreaterThan, flatGain)
			})
		})
	})
}

func TestCalibrationScaleDownsideStaysImmediate(t *testing.T) {
	Convey("Given a healthy scale at its baseline", t, func() {
		scale := newCalibrationScale(DefaultCalibrationParams().gateParams())
		scale.Observe(1, 0.2)
		for i := 0; i < 20; i++ {
			scale.Observe(1, 0.2)
		}

		Convey("Recovery should be closed while healthy", func() {
			So(scale.recoveryOpen(), ShouldBeFalse)

			Convey("A downside sample should still lower the scale immediately", func() {
				before := scale.Scale()
				scale.Observe(0.6, 0.2)
				So(scale.Scale(), ShouldBeLessThan, before)
			})
		})
	})
}
