package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestPaperEligibleRequiresEvidence(t *testing.T) {
	Convey("Setup", t, withTree(func(tree *Tree) {
		path := "R06.R00"
		decision, ok := get(tree, path)
		So(ok, ShouldBeTrue)
		decision.Resistance = system.Cfg.Learning.PaperEligibleResistance
		put(tree, path, decision)

		Convey("When the leaf has no settled samples", func() {
			So(tree.PaperEligible(path, decision.Resistance), ShouldBeFalse)
		})

		Convey("When the leaf has enough winning samples", func() {
			for i := 0; i < system.Cfg.Learning.PaperMinSamples; i++ {
				tree.Observe(path, 1.0+float64(i)*0.1, true)
			}

			Convey("It clears paper eligibility", func() {
				So(tree.PaperEligible(path, decision.Resistance), ShouldBeTrue)
			})
		})

		Convey("When wins are rare despite sample count", func() {
			for i := 0; i < system.Cfg.Learning.PaperMinSamples; i++ {
				tree.Observe(path, -0.5, false)
			}

			Convey("It stays virtual", func() {
				So(tree.PaperEligible(path, decision.Resistance), ShouldBeFalse)
			})
		})
	}))
}
