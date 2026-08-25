package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOvercompleteMultiTimescaleManifold(t *testing.T) {
	Convey("Given an overcomplete multi-timescale architecture [2, 8, 3]", t, func() {
		manifold := NewResonanceManifold([]int{2, 8, 3}, 1, 0.03)

		Convey("The overcomplete layer should have higher sparsity penalty", func() {
			So(manifold.cfg.Sparsity[0], ShouldBeGreaterThan, manifold.cfg.Sparsity[1])
			So(len(manifold.temporalOperators), ShouldEqual, 2)
		})

		Convey("Settling should compute multi-layer readouts with innovations", func() {
			err := manifold.Settle([]float64{0.5, -0.5}, true)
			So(err, ShouldBeNil)

			readout := manifold.ReadoutVector()
			// [z1(8) + z2(3)] + [e0(2) + e1(8)] = 21 dimensions
			So(len(readout), ShouldEqual, 21)
		})

		Convey("Learn should update all multi-timescale temporal matrices and the RLS head", func() {
			err := manifold.Settle([]float64{0.5, -0.5}, false)
			So(err, ShouldBeNil)

			learnErr := manifold.Learn([]float64{0.02})
			So(learnErr, ShouldBeNil)

			pred := manifold.TaskPrediction()
			So(len(pred), ShouldEqual, 1)
		})
	})
}

func TestPerHorizonTaskHead(t *testing.T) {
	Convey("Given a per-horizon task head over architecture [2, 8, 3]", t, func() {
		manifold := NewResonanceManifoldWithHorizon([]int{2, 8, 3}, 1, 4, 0.03)

		Convey("The task head holds one row per horizon", func() {
			So(manifold.taskRows, ShouldEqual, 4)

			pred := manifold.TaskPrediction()
			So(len(pred), ShouldEqual, 4)
		})

		Convey("ObserveTask trains only the addressed horizon row", func() {
			err := manifold.Settle([]float64{0.5, -0.5}, false)
			So(err, ShouldBeNil)

			readout := manifold.ReadoutVector()

			err = manifold.ObserveTask(4, readout, 0.1, 1.0)
			So(err, ShouldBeNil)

			skill, ready := manifold.TaskSkillAt(4)
			So(ready, ShouldBeTrue)
			So(skill, ShouldBeGreaterThan, 0)

			_, unready := manifold.TaskSkillAt(1)
			So(unready, ShouldBeFalse)
		})

		Convey("RolloutTaskForecast returns one cumulative forecast per horizon from the current readout", func() {
			err := manifold.Settle([]float64{0.5, -0.5}, false)
			So(err, ShouldBeNil)

			forecasts, err := manifold.RolloutTaskForecast(4)
			So(err, ShouldBeNil)
			So(len(forecasts), ShouldEqual, 4)

			// Clamping: a request beyond the head's rows yields the head's rows.
			clamped, err := manifold.RolloutTaskForecast(9)
			So(err, ShouldBeNil)
			So(len(clamped), ShouldEqual, 4)
		})

		Convey("An out-of-range task horizon is rejected", func() {
			err := manifold.ObserveTask(5, make([]float64, 21), 0, 1)
			So(err, ShouldNotBeNil)
		})
	})
}
