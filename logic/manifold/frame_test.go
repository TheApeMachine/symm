package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

func TestSolver_paint(t *testing.T) {
	Convey("Given a GPU display frame", t, func() {
		solver := &Solver{}
		state := &State{}
		frame := projectFrame{
			grid:    pfluid.Grid{X: 2, Y: 1, Z: 2, Spacing: 0.5},
			display: []byte{1, 2, 3, 255, 4, 5, 6, 255, 7, 8, 9, 255, 10, 11, 12, 255},
			width:   2,
			height:  2,
			stats: pfluid.DisplayStats{
				Width: 2, Height: 2,
				RhoOccupied: 1, PsiOccupied: 2,
				RhoMax: 3, PsiMax: 4,
			},
			wave: []pfluid.WaveMode{{Omega: 1, Real: 0.2, Imaginary: 0.1}},
		}

		Convey("It attaches display stats and oscillator counts without lattices", func() {
			solver.paint(state, frame, nil, 7)
			So(state.Display, ShouldHaveLength, 16)
			So(state.DisplayWidth, ShouldEqual, 2)
			So(state.DisplayHeight, ShouldEqual, 2)
			So(state.RhoOccupied, ShouldEqual, 1)
			So(state.PsiOccupied, ShouldEqual, 2)
			So(state.RhoMax, ShouldEqual, 3.0)
			So(state.PsiMax, ShouldEqual, 4.0)
			So(state.OscillatorCount, ShouldEqual, 7)
			So(state.SharedOscillatorCount, ShouldEqual, 7)
			So(state.Wave, ShouldHaveLength, 1)
			So(state.PhaseReady, ShouldBeFalse)
			So(state.PhaseReason, ShouldEqual,
				"awaiting a prior outcome-labeled phase observation")
		})
	})
}
