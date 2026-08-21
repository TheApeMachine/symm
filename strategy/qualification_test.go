package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestReserveQualification(t *testing.T) {
	Convey("Given a classified sudden pump with one-horizon predictive support", t, func() {
		qualified, reason := reserveQualification(
			types.OpportunitySuddenPump,
			true,
			1,
		)

		Convey("It should qualify the emergency reserve lane", func() {
			So(qualified, ShouldBeTrue)
			So(reason, ShouldEqual,
				"sudden-pump precursor with one-horizon predictive support")
		})
	})

	Convey("Given a coiled compression with the same predictive state", t, func() {
		qualified, reason := reserveQualification(
			types.OpportunityCoiledCompression,
			true,
			1,
		)

		Convey("It should remain a normal opportunity", func() {
			So(qualified, ShouldBeFalse)
			So(reason, ShouldEqual,
				"structural opportunity is not an emergency reserve setup")
		})
	})
}
