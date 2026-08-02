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
		})
	})
}
