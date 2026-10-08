package geometry_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestGeometry(t *testing.T) {
	Convey("Given geometry primitives", t, func() {
		Convey("Corpus computes cosine similarity and maintains bounded history", func() {
			corpus := geometry.NewCorpus(2)
			var results []float64

			for ptr := range corpus.Next(data.NewValue(1.0, 0.0).Next(nil)) {
				results = append(results, *(*float64)(ptr))
			}

			So(corpus.Error(), ShouldBeNil)
			So(len(results), ShouldEqual, 2)
			So(results[0], ShouldEqual, 0.0) // similarity against empty history
			So(results[1], ShouldEqual, 1.0) // size

			results = nil

			for ptr := range corpus.Next(data.NewValue(1.0, 0.0).Next(nil)) {
				results = append(results, *(*float64)(ptr))
			}

			So(corpus.Error(), ShouldBeNil)
			So(len(results), ShouldEqual, 2)
			So(results[0], ShouldAlmostEqual, 1.0, 1e-6)
			So(results[1], ShouldEqual, 2.0)
		})

		Convey("Normalize projects coordinates to unit sphere", func() {
			normalize := geometry.NewNormalize()
			var results []float64

			for ptr := range normalize.Next(data.NewValue(3.0, 4.0).Next(nil)) {
				results = append(results, *(*float64)(ptr))
			}

			So(normalize.Error(), ShouldBeNil)
			So(len(results), ShouldEqual, 3)
			So(results[0], ShouldAlmostEqual, 0.6, 1e-6)
			So(results[1], ShouldAlmostEqual, 0.8, 1e-6)
			So(results[2], ShouldAlmostEqual, 5.0, 1e-6)
		})

		Convey("Overlap computes dot product, affinity, and distance", func() {
			overlap := geometry.NewOverlap()
			var results []float64

			for ptr := range overlap.Next(data.NewValue(1.0, 0.0, 0.0, 1.0).Next(nil)) {
				results = append(results, *(*float64)(ptr))
			}

			So(overlap.Error(), ShouldBeNil)
			So(len(results), ShouldEqual, 3)
			So(results[0], ShouldEqual, 0.0) // dot
			So(results[1], ShouldEqual, 0.0) // affinity (orthogonal)
			So(results[2], ShouldAlmostEqual, 1.41421356, 1e-5) // distance sqrt(2)
		})

		Convey("PhasePath generates angular positions", func() {
			phase := geometry.NewPhasePath()
			var results []float64

			for ptr := range phase.Next(data.NewValue(4.0).Next(nil)) {
				results = append(results, *(*float64)(ptr))
			}

			So(phase.Error(), ShouldBeNil)
			So(len(results), ShouldEqual, 2)
			So(results[0], ShouldEqual, 0.0) // angle 0
			So(results[1], ShouldEqual, 0.0) // phase 0
		})

		Convey("Relaxation performs displacement towards target", func() {
			relaxation := geometry.NewRelaxation()
			var results []float64

			for ptr := range relaxation.Next(data.NewValue(0.0, 0.0, 2.0, 0.0, 0.5, 1.0).Next(nil)) {
				results = append(results, *(*float64)(ptr))
			}

			So(relaxation.Error(), ShouldBeNil)
			So(len(results), ShouldEqual, 3)
			So(results[0], ShouldBeGreaterThan, 0.0)
		})

		Convey("Watershed assigns basin and detects authority peaks", func() {
			watershed := geometry.NewWatershed()
			var results []float64

			for ptr := range watershed.Next(data.NewValue(0.8, 0.8, 10.0).Next(nil)) {
				results = append(results, *(*float64)(ptr))
			}

			So(watershed.Error(), ShouldBeNil)
			So(len(results), ShouldEqual, 2)
			So(results[0], ShouldEqual, 3.0) // basin (both >= 0.5 -> 1 + 2 = 3)
			So(results[1], ShouldEqual, 1.0) // peak (first observation)
		})
	})
}
