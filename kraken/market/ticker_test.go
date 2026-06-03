package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTickerUpdateSetEnvelopeType(t *testing.T) {
	Convey("Given a ticker update", t, func() {
		ticker := TickerUpdate{Symbol: "BTC/EUR", Last: 100}

		ticker.SetEnvelopeType("snapshot")

		Convey("It should record the envelope type", func() {
			So(ticker.Type, ShouldEqual, "snapshot")
		})
	})
}
