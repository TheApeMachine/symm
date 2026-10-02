package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSkill(t *testing.T) {
	Convey("Skill tracker", t, func() {
		skill := NewSkill()

		Convey("Initial state has zero resolved and no edge", func() {
			So(skill.Resolved(), ShouldEqual, 0)
			So(skill.WinRate(), ShouldEqual, 0)
			So(skill.Edge(), ShouldEqual, 0)
			So(skill.HasEdge(), ShouldBeFalse)
		})

		Convey("Tracks win-rate and edge correctly", func() {
			// Record 6 wins of 2% and 4 losses of -1%
			for i := 0; i < 6; i++ {
				skill.Record(0.02)
			}
			for i := 0; i < 4; i++ {
				skill.Record(-0.01)
			}

			So(skill.Resolved(), ShouldEqual, 10)
			So(skill.WinRate(), ShouldAlmostEqual, 0.60, 1e-4)
			// Total edge: (6*0.02 - 4*0.01) / 10 = (0.12 - 0.04) / 10 = 0.008 (80 bp)
			So(skill.Edge(), ShouldAlmostEqual, 0.008, 1e-4)
			So(skill.HasEdge(), ShouldBeTrue)
		})

		Convey("Requires minimum sample count before demonstrating edge", func() {
			// Record 4 consecutive big wins
			for i := 0; i < 4; i++ {
				skill.Record(0.05)
			}

			So(skill.Resolved(), ShouldEqual, 4)
			So(skill.WinRate(), ShouldAlmostEqual, 1.0, 1e-4)
			// Not enough samples yet to prove edge
			So(skill.HasEdge(), ShouldBeFalse)
		})

		Convey("Rejects negative edge despite high win rate if net returns are negative", func() {
			// 6 small wins of 0.001 and 4 huge losses of -0.05
			for i := 0; i < 6; i++ {
				skill.Record(0.001)
			}
			for i := 0; i < 4; i++ {
				skill.Record(-0.05)
			}

			So(skill.Resolved(), ShouldEqual, 10)
			So(skill.WinRate(), ShouldAlmostEqual, 0.60, 1e-4)
			So(skill.Edge(), ShouldBeLessThan, 0)
			So(skill.HasEdge(), ShouldBeFalse)
		})
	})
}
