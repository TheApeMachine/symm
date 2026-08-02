package tests

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSymbolNewSymbol(t *testing.T) {
	Convey("Given symbol configuration", t, func() {
		symbol := NewSymbol("SIM1/USD", 100.0, 42)

		Convey("It should instantiate Symbol with an active generator", func() {
			So(symbol, ShouldNotBeNil)
			So(symbol.pair, ShouldEqual, "SIM1/USD")
			So(symbol.startPrice, ShouldEqual, 100.0)
			So(symbol.generator, ShouldNotBeNil)
		})
	})
}
