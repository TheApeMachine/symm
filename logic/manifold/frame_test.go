package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

func TestProjectionRows(t *testing.T) {
	Convey("Given a flattened X-Z field", t, func() {
		grid := pfluid.Grid{X: 2, Y: 1, Z: 2, Spacing: 0.5}

		Convey("It should preserve row-major physical values", func() {
			So(projectionRows([]float32{1, 2, 3, 4}, grid), ShouldResemble,
				[][]float64{{1, 2}, {3, 4}})
		})
	})
}

func TestRenderParticles(t *testing.T) {
	Convey("Given a post-step physical observation", t, func() {
		grid := pfluid.Grid{X: 2, Y: 2, Z: 2, Spacing: 0.5}
		particles := []pfluid.Particle{{
			Position: pfluid.Vector{X: 0.25, Y: 0.5, Z: 0.75},
			Velocity: pfluid.Vector{X: 3, Y: 4},
			Energy:   9,
		}}
		rendered := renderParticles(particles, grid)

		Convey("It should expose cell position, oscillator amplitude, and speed", func() {
			So(rendered, ShouldHaveLength, 1)
			So(rendered[0].CellX, ShouldEqual, 0.5)
			So(rendered[0].CellY, ShouldEqual, 1)
			So(rendered[0].CellZ, ShouldEqual, 1.5)
			So(rendered[0].Amplitude, ShouldEqual, 3)
			So(rendered[0].Speed, ShouldEqual, 5)
		})
	})
}
