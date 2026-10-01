package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSpotSymbolForFuturesAndPreferPerpetual(t *testing.T) {
	Convey("Futures instrument maps onto spot WS symbols", t, func() {
		So(SpotSymbolForFutures(FuturesInstrument{Base: "BTC", Quote: "USD"}), ShouldEqual, "BTC/USD")
		So(SpotSymbolForFutures(FuturesInstrument{Pair: "ETH:USD"}), ShouldEqual, "ETH/USD")
	})

	Convey("PreferPerpetual favors PF_ over PI_", t, func() {
		got := PreferPerpetual([]FuturesInstrument{
			{Symbol: "PI_XBTUSD", Tradeable: true},
			{Symbol: "PF_XBTUSD", Tradeable: true},
			{Symbol: "FI_XBTUSD_210625", Tradeable: true},
		})
		So(got, ShouldEqual, "PF_XBTUSD")
	})

	Convey("PreferPerpetual ignores non-tradeable listings", t, func() {
		got := PreferPerpetual([]FuturesInstrument{
			{Symbol: "PF_XBTUSD", Tradeable: false},
			{Symbol: "PI_XBTUSD", Tradeable: true},
		})
		So(got, ShouldEqual, "PI_XBTUSD")
	})
}
