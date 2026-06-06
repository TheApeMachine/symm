package reasoning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestIsShortAct(t *testing.T) {
	Convey("Given reasoning acts", t, func() {
		Convey("It should treat sell limit entries as short acts", func() {
			So(IsShortAct(Act{Type: ActionLimit, Side: trading.Sell}), ShouldBeTrue)
		})

		Convey("It should treat default limit entries as not short", func() {
			So(IsShortAct(Act{Type: ActionLimit}), ShouldBeFalse)
		})

		Convey("It should treat protective exits as not short entries", func() {
			So(IsShortAct(Act{Type: ActionStopLoss, Side: trading.Sell}), ShouldBeFalse)
		})
	})
}
