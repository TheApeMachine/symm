package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestOvercompleteMultiTimescaleManifold(t *testing.T) {
	Convey("Given an overcomplete multi-timescale architecture [2, 8, 3]", t, func() {
		manifold := NewResonanceManifold([]int{2, 8, 3}, 1, 1, 0.03, ReadoutAll).(*ResonanceManifold)

		Convey("The overcomplete layer should have higher sparsity penalty", func() {
			So(manifold.cfg.Sparsity[0], ShouldBeGreaterThan, manifold.cfg.Sparsity[1])
			So(len(manifold.temporalOperators), ShouldEqual, 2)
		})

		Convey("Settling should compute multi-layer readouts with innovations", func() {
			reading := data.Read[[10][]float64](manifold.Next(data.NewValue(
				[3][]float64{{ManifoldSettle, 1}, {0.5, -0.5}, nil},
			).Next(nil)))
			So(manifold.Error(), ShouldBeNil)

			// [z1(8) + z2(3)] + [e0(2) + e1(8)] = 21 dimensions
			So(reading[0][8], ShouldEqual, 21)
			So(len(reading[1]), ShouldEqual, 21)
		})

		Convey("Learn should update all multi-timescale temporal matrices and the RLS head", func() {
			data.Read[[10][]float64](manifold.Next(data.NewValue(
				[3][]float64{{ManifoldSettle}, {0.5, -0.5}, nil},
			).Next(nil)))
			So(manifold.Error(), ShouldBeNil)

			reading := data.Read[[10][]float64](manifold.Next(data.NewValue(
				[3][]float64{{ManifoldLearn}, {0.02}, nil},
			).Next(nil)))
			So(manifold.Error(), ShouldBeNil)
			So(reading[3], ShouldHaveLength, 1)
		})
	})
}

func TestPerHorizonTaskHead(t *testing.T) {
	Convey("Given a per-horizon task head over architecture [2, 8, 3]", t, func() {
		manifold := NewResonanceManifold([]int{2, 8, 3}, 1, 4, 0.03, ReadoutAll).(*ResonanceManifold)
		settle := [3][]float64{{ManifoldSettle}, {0.5, -0.5}, nil}
		read := [3][]float64{{ManifoldReading}, nil, nil}

		Convey("The task head holds one row per horizon", func() {
			So(manifold.taskRows, ShouldEqual, 4)

			reading := data.Read[[10][]float64](manifold.Next(data.NewValue(read).Next(nil)))
			So(reading[3], ShouldHaveLength, 4)
		})

		Convey("A task observation trains only the addressed horizon row", func() {
			reading := data.Read[[10][]float64](manifold.Next(data.NewValue(settle).Next(nil)))
			So(manifold.Error(), ShouldBeNil)

			data.Read[[10][]float64](manifold.Next(data.NewValue(
				[3][]float64{{ManifoldTask, 4, 0.1, 1.0}, reading[1], nil},
			).Next(nil)))
			So(manifold.Error(), ShouldBeNil)

			snapshot := data.Read[[10][]float64](manifold.Next(data.NewValue(read).Next(nil)))
			So(snapshot[5][3], ShouldEqual, 1)
			So(snapshot[4][3], ShouldBeGreaterThan, 0)
			So(snapshot[5][0], ShouldEqual, 0)
		})

		Convey("A forecast returns one cumulative forecast per horizon from the current readout", func() {
			data.Read[[10][]float64](manifold.Next(data.NewValue(settle).Next(nil)))
			So(manifold.Error(), ShouldBeNil)

			reading := data.Read[[10][]float64](manifold.Next(data.NewValue(
				[3][]float64{{ManifoldForecast, 4}, nil, nil},
			).Next(nil)))
			So(manifold.Error(), ShouldBeNil)
			So(reading[8], ShouldHaveLength, 4*6)

			// Clamping: a request beyond the head's rows yields the head's rows.
			clamped := data.Read[[10][]float64](manifold.Next(data.NewValue(
				[3][]float64{{ManifoldForecast, 9}, nil, nil},
			).Next(nil)))
			So(manifold.Error(), ShouldBeNil)
			So(clamped[8], ShouldHaveLength, 4*6)
		})

		Convey("An out-of-range task horizon is rejected", func() {
			for range manifold.Next(data.NewValue(
				[3][]float64{{ManifoldTask, 5, 0, 1}, make([]float64, 21), nil},
			).Next(nil)) {
			}

			So(manifold.Error(), ShouldNotBeNil)
		})
	})
}

/*
TestManifoldPrimitiveWire proves the manifold answers through its wire as a
core.Primitive.
*/
func TestManifoldPrimitiveWire(t *testing.T) {
	Convey("Given a manifold primitive on the wire", t, func() {
		var manifold core.Primitive = NewResonanceManifold([]int{2, 4, 2}, 1, 2, 0.05, ReadoutAll)

		Convey("A settle command yields exactly one reading", func() {
			readings := 0
			var reading [10][]float64

			for out := range manifold.Next(data.NewValue(
				[3][]float64{{ManifoldSettle}, {0.5, -0.5}, nil},
			).Next(nil)) {
				reading = *(*[10][]float64)(out)
				readings++
			}

			So(manifold.Error(), ShouldBeNil)
			So(readings, ShouldEqual, 1)
			So(reading[0][8], ShouldEqual, 12)
			// Three layers, each {errorNorm, temporal, state..., prediction...}.
			So(reading[7], ShouldHaveLength, (2+2*2)+(2+2*4)+(2+2*2))
		})

		Convey("A rejected architecture yields nothing and records its error", func() {
			rejected := NewResonanceManifold([]int{2}, 1, 2, 0.05, ReadoutAll)

			for range rejected.Next(data.NewValue(
				[3][]float64{{ManifoldSettle}, {0.5}, nil},
			).Next(nil)) {
				t.Fatal("rejected manifold must yield nothing")
			}

			So(rejected.Error(), ShouldNotBeNil)
		})
	})
}
