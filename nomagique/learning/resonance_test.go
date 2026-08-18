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
