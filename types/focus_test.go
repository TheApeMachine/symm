package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFocus(t *testing.T) {
	Convey("Focus gates wire symbols", t, func() {
		prev := Focus()
		Reset(func() { SetFocus(prev) })

		Convey("default Allows matches stored focus", func() {
			SetFocus("BTC/USD")
			So(Focus(), ShouldEqual, "BTC/USD")
			So(Allows("BTC/USD"), ShouldBeTrue)
			So(Allows("ETH/USD"), ShouldBeFalse)
		})

		Convey("empty focus suppresses every symbol", func() {
			SetFocus("")
			So(Focus(), ShouldEqual, "")
			So(Allows("BTC/USD"), ShouldBeFalse)
			So(Allows(""), ShouldBeFalse)
		})
	})
}
