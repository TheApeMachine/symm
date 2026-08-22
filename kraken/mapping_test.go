package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSpotToFuturesProductID(t *testing.T) {
	Convey("Given spot market symbols", t, func() {
		Convey("It should correctly map BTC/USD to PF_XBTUSD", func() {
			So(SpotToFuturesProductID("BTC/USD"), ShouldEqual, "PF_XBTUSD")
		})

		Convey("It should correctly map ZEC/USD to PF_ZECUSD", func() {
			So(SpotToFuturesProductID("ZEC/USD"), ShouldEqual, "PF_ZECUSD")
		})

		Convey("It should correctly map ETH/USD to PF_ETHUSD", func() {
			So(SpotToFuturesProductID("ETH/USD"), ShouldEqual, "PF_ETHUSD")
		})

		Convey("It should correctly map DOGE/USD to PF_XDGUSD", func() {
			So(SpotToFuturesProductID("DOGE/USD"), ShouldEqual, "PF_XDGUSD")
		})

		Convey("It should return empty string for invalid symbol", func() {
			So(SpotToFuturesProductID("INVALID"), ShouldEqual, "")
			So(SpotToFuturesProductID(""), ShouldEqual, "")
		})
	})
}

func TestFuturesProductIDToSpot(t *testing.T) {
	Convey("Given futures product identifiers", t, func() {
		Convey("It should correctly map PF_XBTUSD to BTC/USD", func() {
			So(FuturesProductIDToSpot("PF_XBTUSD"), ShouldEqual, "BTC/USD")
		})

		Convey("It should correctly map PI_XBTUSD to BTC/USD", func() {
			So(FuturesProductIDToSpot("PI_XBTUSD"), ShouldEqual, "BTC/USD")
		})

		Convey("It should correctly map PF_ZECUSD to ZEC/USD", func() {
			So(FuturesProductIDToSpot("PF_ZECUSD"), ShouldEqual, "ZEC/USD")
		})

		Convey("It should correctly map PF_ETHUSD to ETH/USD", func() {
			So(FuturesProductIDToSpot("PF_ETHUSD"), ShouldEqual, "ETH/USD")
		})

		Convey("It should return empty string for unparseable product ID", func() {
			So(FuturesProductIDToSpot("INVALID"), ShouldEqual, "")
			So(FuturesProductIDToSpot(""), ShouldEqual, "")
		})
	})
}

func BenchmarkSpotToFuturesProductID(b *testing.B) {
	for b.Loop() {
		_ = SpotToFuturesProductID("BTC/USD")
	}
}

func BenchmarkFuturesProductIDToSpot(b *testing.B) {
	for b.Loop() {
		_ = FuturesProductIDToSpot("PF_XBTUSD")
	}
}
