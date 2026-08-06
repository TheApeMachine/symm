package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSymbolNewSymbol(t *testing.T) {
	Convey("Given symbol configuration", t, func() {
		symbol := NewSymbol("SIM1/USD", 100.0, 42)

		Convey("It should instantiate Symbol with an active generator", func() {
			So(symbol, ShouldNotBeNil)
			So(symbol.Pair, ShouldEqual, "SIM1/USD")
			So(symbol.StartPrice, ShouldEqual, 100.0)
			So(symbol.PriceIncrement, ShouldEqual, 0.01)
			So(symbol.PricePrecision, ShouldEqual, 2)
		})
	})

	Convey("Given symbols at materially different price scales", t, func() {
		low := NewSymbol("LOW/USD", 0.00012345, 1)
		high := NewSymbol("HIGH/USD", 987654321.0, 2)

		Convey("They should retain the same significant quoting resolution", func() {
			So(low.PriceIncrement, ShouldEqual, 0.00000001)
			So(low.PricePrecision, ShouldEqual, 8)
			So(high.PriceIncrement, ShouldEqual, 10000.0)
			So(high.PricePrecision, ShouldEqual, 0)
		})
	})

	Convey("Given an invalid start price", t, func() {
		Convey("It should reject a symbol whose book cannot carry a price", func() {
			So(func() { NewSymbol("ZERO/USD", 0, 3) }, ShouldPanic)
		})
	})
}
