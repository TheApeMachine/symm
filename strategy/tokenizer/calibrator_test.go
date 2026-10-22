package tokenizer

import (
	"math"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestCalibrator(t *testing.T) {
	Convey("Given a new Calibrator", t, func() {
		cal := NewCalibrator()

		Convey("It should initialize with default sensitivities", func() {
			So(cal.SensitivityPop(), ShouldEqual, 1.0)
			So(cal.SensitivityPhase(), ShouldEqual, 1.0)
			So(cal.window, ShouldNotBeNil)
			So(cal.phaseWindow, ShouldNotBeNil)
		})

		Convey("When testing concurrent access to getters and setters", func() {
			var wg sync.WaitGroup
			for i := range 100 {
				wg.Add(2)
				go func(val float64) {
					defer wg.Done()
					cal.SetSensitivityPop(val)
					_ = cal.SensitivityPop()
				}(float64(i))
				go func(val float64) {
					defer wg.Done()
					cal.SetSensitivityPhase(val)
					_ = cal.SensitivityPhase()
				}(float64(i))
			}
			wg.Wait()

			Convey("It should not panic or cause race conditions", func() {
				So(cal.SensitivityPop(), ShouldBeGreaterThanOrEqualTo, 0.0)
				So(cal.SensitivityPhase(), ShouldBeGreaterThanOrEqualTo, 0.0)
			})
		})

		Convey("When recalibrating with chunk density", func() {
			cal = NewCalibrator(WithWindowSize(5))

			Convey("It should early return if window is not warmed", func() {
				cal.FeedbackChunk(0.35, 1.0, 1.0)
				So(cal.SensitivityPop(), ShouldEqual, 1.0)
				So(cal.SensitivityPhase(), ShouldEqual, 1.0)
			})

			Convey("It should early return if mean density is zero", func() {
				for range 5 {
					cal.FeedbackChunk(0.0, 1.0, 1.0)
				}
				So(cal.SensitivityPop(), ShouldEqual, 1.0)
				So(cal.SensitivityPhase(), ShouldEqual, 1.0)
			})

			Convey("It should adjust sensitivities correctly given chunk density", func() {
				cal.FeedbackChunk(0.10, 1.0, 1.0)
				cal.FeedbackChunk(0.15, 1.0, 1.0)
				cal.FeedbackChunk(0.20, 1.0, 1.0)
				cal.FeedbackChunk(0.10, 1.0, 1.0)
				cal.FeedbackChunk(0.20, 1.0, 1.0)

				pop := cal.SensitivityPop()
				phase := cal.SensitivityPhase()

				So(pop, ShouldBeGreaterThan, 1.0)
				So(phase, ShouldBeGreaterThan, 1.0)

				for range 2 {
					cal.FeedbackChunk(0.60, 1.0, 1.0)
					cal.FeedbackChunk(0.65, 1.0, 1.0)
					cal.FeedbackChunk(0.70, 1.0, 1.0)
					cal.FeedbackChunk(0.60, 1.0, 1.0)
					cal.FeedbackChunk(0.70, 1.0, 1.0)
				}

				So(cal.SensitivityPop(), ShouldBeLessThan, pop)
				So(cal.SensitivityPhase(), ShouldBeLessThan, phase)
			})

			Convey("It should derive dynamic ceilings from warmed density and phase history", func() {
				cal.FeedbackChunk(0.10, 1.0, 1.0)
				cal.FeedbackChunk(0.15, 1.0, 1.0)
				cal.FeedbackChunk(0.20, 1.0, 1.0)
				cal.FeedbackChunk(0.10, 1.0, 1.0)
				cal.FeedbackChunk(0.20, 1.0, 1.0)

				cal.ObservePhase(0.20)
				cal.ObservePhase(0.25)
				cal.ObservePhase(0.30)
				cal.ObservePhase(0.35)
				cal.ObservePhase(0.40)

				So(cal.DensityCeiling(
					system.Cfg.Learning.ShannonCapacity,
				), ShouldBeLessThan, system.Cfg.Learning.VocabSize)
				So(cal.PhaseLimit(math.Pi/2.0), ShouldBeLessThan, math.Pi/2.0)
			})
		})
	})
}

func BenchmarkFeedbackChunk(b *testing.B) {
	cal := NewCalibrator(WithWindowSize(128))

	for b.Loop() {
		cal.FeedbackChunk(0.35, 1.0, 1.0)
	}
}
