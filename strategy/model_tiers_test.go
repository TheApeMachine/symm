package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestEnterTierRouting(t *testing.T) {
	Convey("High resistance stays virtual; low resistance is paper-eligible", t, func() {
		prev := system.Cfg
		system.Cfg = &system.Config{
			Learning: &system.Learning{PaperEligibleResistance: 0.2},
			Market:   &system.Market{Model: "real"},
		}
		defer func() { system.Cfg = prev }()

		So(PaperEligible(0.25), ShouldBeFalse)
		So(PaperEligible(0.2), ShouldBeTrue)

		confidence := &Confidence{armed: true}
		So(confidence.Armed(), ShouldBeTrue)

		confidence.armed = false
		So(confidence.Armed(), ShouldBeFalse)
	})
}
