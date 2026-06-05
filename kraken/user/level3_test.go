package user

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

func TestNewLevel3SubscribeFrame(t *testing.T) {
	Convey("Given symbols and a token", t, func() {
		frame := NewLevel3SubscribeFrame([]string{"BTC/EUR"}, 10, "token")

		Convey("It should target the level3 channel", func() {
			So(frame.Method, ShouldEqual, "subscribe")
			So(frame.Params.Channel, ShouldEqual, public.Level3Channel)
			So(frame.Params.Symbol, ShouldResemble, []string{"BTC/EUR"})
			So(frame.Params.Depth, ShouldEqual, 10)
			So(frame.Params.Token, ShouldEqual, "token")
		})
	})
}
