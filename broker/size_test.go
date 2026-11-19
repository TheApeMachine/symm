package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestSizeHintConviction(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the hint is zero-value", func() {
			So(SizeHint{}.Conviction(), ShouldEqual, 1)
		})

		Convey("When a matched high-score path sizes up", func() {
			hint := SizeHint{
				Score:      system.Cfg.Selection.MinBoardScore + 1,
				Resistance: 0.05,
				PathFound:  true,
			}
			So(hint.Conviction(), ShouldBeGreaterThan, 0.55)
			So(hint.Conviction(), ShouldBeLessThanOrEqualTo, 1)
		})

		Convey("When PathFound alone has no score", func() {
			So(SizeHint{PathFound: true}.Conviction(), ShouldEqual, 1)
		})

		Convey("When explore resistance is high", func() {
			hint := SizeHint{Score: 0.1, Resistance: 0.9, PathFound: false}
			So(hint.Conviction(), ShouldBeGreaterThanOrEqualTo, 0.35)
			So(hint.Conviction(), ShouldBeLessThan, 0.55)
		})
	})
}
