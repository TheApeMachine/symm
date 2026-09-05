package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestSpotSymbol pins the resolution the basis reading depends on.

Kraken names the same market twice and states the relationship nowhere. A
consumer accumulating evidence per symbol files derivative facts under the
product identity and spot facts under the pair, so without this the two never
meet and any advisor requiring both is mute on every instrument.
*/
func TestSpotSymbol(t *testing.T) {
	Convey("Given Kraken futures product identities", t, func() {
		Convey("a perpetual resolves to its spot pair", func() {
			symbol, ok := SpotSymbol("PF_SOLUSD")

			So(ok, ShouldBeTrue)
			So(symbol, ShouldEqual, "SOL/USD")
		})

		Convey("a numeric-leading base survives intact", func() {
			symbol, ok := SpotSymbol("PF_1INCHUSD")

			So(ok, ShouldBeTrue)
			So(symbol, ShouldEqual, "1INCH/USD")
		})

		Convey("the venues' disagreeing asset names are reconciled", func() {
			symbol, ok := SpotSymbol("PF_XBTUSD")

			So(ok, ShouldBeTrue)
			So(symbol, ShouldEqual, "BTC/USD")
		})

		Convey("a stablecoin quote is not truncated to its prefix", func() {
			symbol, ok := SpotSymbol("PF_ETHUSDT")

			So(ok, ShouldBeTrue)
			So(symbol, ShouldEqual, "ETH/USDT")
		})

		Convey("an inverse perpetual resolves the same way", func() {
			symbol, ok := SpotSymbol("PI_XBTUSD")

			So(ok, ShouldBeTrue)
			So(symbol, ShouldEqual, "BTC/USD")
		})

		Convey("a spot symbol is refused rather than mangled", func() {
			_, ok := SpotSymbol("SOL/USD")

			So(ok, ShouldBeFalse)
		})

		Convey("an unrecognised quote is refused rather than guessed", func() {
			_, ok := SpotSymbol("PF_SOLJPY")

			So(ok, ShouldBeFalse)
		})

		Convey("a product with no base is refused", func() {
			_, ok := SpotSymbol("PF_USD")

			So(ok, ShouldBeFalse)
		})
	})
}
