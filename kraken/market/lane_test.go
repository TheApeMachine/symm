package market

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestSpotIdentityFromPair(t *testing.T) {
	convey.Convey("Given a spot pair symbol", t, func() {
		convey.Convey("It should map BTC to XBT on the spot lane", func() {
			identity, err := SpotIdentityFromPair("BTC/USD")

			convey.So(err, convey.ShouldBeNil)
			convey.So(identity.Base, convey.ShouldEqual, "XBT")
			convey.So(identity.Lane, convey.ShouldEqual, InstrumentLaneSpot)
		})
	})
}

func TestFuturesIdentityFromProduct(t *testing.T) {
	convey.Convey("Given a Kraken futures product id", t, func() {
		convey.Convey("It should classify inverse perpetuals on the perpetual lane", func() {
			identity, err := FuturesIdentityFromProduct("PI_XBTUSD")

			convey.So(err, convey.ShouldBeNil)
			convey.So(identity.Base, convey.ShouldEqual, "XBT")
			convey.So(identity.Lane, convey.ShouldEqual, InstrumentLanePerpetual)
		})

		convey.Convey("It should classify dated futures on the dated lane", func() {
			identity, err := FuturesIdentityFromProduct("FI_ETHUSD_210625")

			convey.So(err, convey.ShouldBeNil)
			convey.So(identity.Base, convey.ShouldEqual, "ETH")
			convey.So(identity.Lane, convey.ShouldEqual, InstrumentLaneDatedFuture)
		})
	})
}

func TestPerpetualProductFromSpotPair(t *testing.T) {
	convey.Convey("Given a USD spot pair", t, func() {
		convey.Convey("It should derive the inverse perpetual product id", func() {
			productID, err := PerpetualProductFromSpotPair("XBT/USD")

			convey.So(err, convey.ShouldBeNil)
			convey.So(productID, convey.ShouldEqual, "PI_XBTUSD")
		})
	})
}
