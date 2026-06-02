package market

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestInstrumentPairScannable(t *testing.T) {
	convey.Convey("Given discovery mode with quote currency EUR", t, func() {
		convey.Convey("It should accept every online EUR pair", func() {
			convey.So(instrumentPairScannable(
				InstrumentPair{Symbol: "PEPE/EUR", Quote: "EUR", Status: "online"},
				nil, "EUR",
			), convey.ShouldBeTrue)
		})

		convey.Convey("It should reject other quote currencies", func() {
			convey.So(instrumentPairScannable(
				InstrumentPair{Symbol: "BTC/USD", Quote: "USD", Status: "online"},
				nil, "EUR",
			), convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given an explicit watchlist", t, func() {
		watched := []string{"PEPE/EUR"}

		convey.Convey("It should only allow listed symbols", func() {
			convey.So(instrumentPairScannable(
				InstrumentPair{Symbol: "PEPE/EUR", Quote: "EUR", Status: "online"},
				watched, "EUR",
			), convey.ShouldBeTrue)

			convey.So(instrumentPairScannable(
				InstrumentPair{Symbol: "ETH/EUR", Quote: "EUR", Status: "online"},
				watched, "EUR",
			), convey.ShouldBeFalse)
		})
	})
}
