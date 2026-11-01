package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestPaperEligible(t *testing.T) {
	Convey("PaperEligible uses the configured resistance floor", t, func() {
		prev := system.Cfg
		system.Cfg = &system.Config{
			Learning: &system.Learning{PaperEligibleResistance: 0.2},
			Market:   &system.Market{Model: "paper"},
		}
		defer func() { system.Cfg = prev }()

		So(PaperEligible(0.21), ShouldBeFalse)
		So(PaperEligible(0.2), ShouldBeTrue)
		So(PaperEligible(0.05), ShouldBeTrue)
		So(PaperEligibleResistance(), ShouldEqual, 0.2)
	})
}

func TestRealCeilingAllowed(t *testing.T) {
	Convey("RealCeilingAllowed follows market.model", t, func() {
		prev := system.Cfg
		defer func() { system.Cfg = prev }()

		system.Cfg = &system.Config{Market: &system.Market{Model: "paper"}}
		So(RealCeilingAllowed(), ShouldBeFalse)

		system.Cfg = &system.Config{Market: &system.Market{Model: "real"}}
		So(RealCeilingAllowed(), ShouldBeTrue)
	})
}
