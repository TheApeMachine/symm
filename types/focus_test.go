package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSetFocus(t *testing.T) {
	Convey("Setup", t, func() {
		prev := Focus()
		Reset(func() { SetFocus(prev) })

		Convey("When SetFocus records BTC/USD", func() {
			SetFocus("BTC/USD")

			Convey("Focus returns BTC/USD", func() {
				So(Focus(), ShouldEqual, "BTC/USD")
			})
		})
	})
}

func TestFocus(t *testing.T) {
	Convey("Setup", t, func() {
		prev := Focus()
		Reset(func() { SetFocus(prev) })

		Convey("When SetFocus records ETH/USD", func() {
			SetFocus("ETH/USD")

			Convey("Focus returns ETH/USD", func() {
				So(Focus(), ShouldEqual, "ETH/USD")
			})
		})
	})
}

func TestAllows(t *testing.T) {
	Convey("Setup", t, func() {
		prev := Focus()
		Reset(func() { SetFocus(prev) })

		Convey("When focus is BTC/USD", func() {
			SetFocus("BTC/USD")

			Convey("only the focused symbol is allowed", func() {
				So(Allows("BTC/USD"), ShouldBeTrue)
				So(Allows("ETH/USD"), ShouldBeFalse)
			})
		})

		Convey("When focus is empty", func() {
			SetFocus("")

			Convey("every symbol is suppressed", func() {
				So(Allows("BTC/USD"), ShouldBeFalse)
				So(Allows(""), ShouldBeFalse)
			})
		})
	})
}
