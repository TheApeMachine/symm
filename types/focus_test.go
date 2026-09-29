package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFocus(t *testing.T) {
	Convey("Given the focus symbol manager", t, func() {
		original := Focus()
		Reset(func() {
			SetFocus(original)
		})

		Convey("Default focus is BTC/USD", func() {
			SetFocus("BTC/USD")
			So(Focus(), ShouldEqual, "BTC/USD")
		})

		Convey("Allows admits matching focus symbols and drops non-matching", func() {
			SetFocus("BTC/USD")
			So(Allows("BTC/USD"), ShouldBeTrue)
			So(Allows("btc/usd"), ShouldBeTrue)
			So(Allows("ETH/USD"), ShouldBeFalse)
			So(Allows("SOL/USD"), ShouldBeFalse)
		})

		Convey("Allows admits global and empty symbol measurements unconditionally", func() {
			SetFocus("BTC/USD")
			So(Allows(""), ShouldBeTrue)
		})

		Convey("Empty focus fails closed and drops symbol measurements", func() {
			SetFocus("")
			So(Allows("BTC/USD"), ShouldBeFalse)
			So(Allows("ETH/USD"), ShouldBeFalse)
			So(Allows(""), ShouldBeTrue)
		})

		Convey("Explicit wildcard or all admits all symbols", func() {
			SetFocus("*")
			So(Allows("BTC/USD"), ShouldBeTrue)
			So(Allows("ETH/USD"), ShouldBeTrue)

			SetFocus("all")
			So(Allows("BTC/USD"), ShouldBeTrue)
			So(Allows("ETH/USD"), ShouldBeTrue)
		})
	})
}
