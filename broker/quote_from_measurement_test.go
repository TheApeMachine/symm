package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestQuoteFromMeasurement(t *testing.T) {
	Convey("Given a measurement with bid, ask, and timestamp", t, func() {
		at := time.Unix(1_700_000_000, 0).UTC()
		quote := QuoteFromMeasurement(types.Measurement{
			Symbol:    "BTC/EUR",
			Bid:       99,
			Ask:       101,
			Last:      100,
			SpreadBPS: 200,
			At:        at,
		})

		So(quote.Symbol, ShouldEqual, "BTC/EUR")
		So(quote.Bid, ShouldEqual, 99)
		So(quote.Ask, ShouldEqual, 101)
		So(quote.UpdatedAt, ShouldEqual, at)
	})

	Convey("Given a measurement with last and spread but no bid/ask", t, func() {
		spreadAt := time.Unix(1_700_000_001, 0).UTC()
		quote := QuoteFromMeasurement(types.Measurement{
			Symbol:    "FXS/EUR",
			Last:      4.36,
			SpreadBPS: 400,
			At:        spreadAt,
		})

		Convey("It should derive bid/ask from spread bps", func() {
			So(quote.Bid, ShouldAlmostEqual, 4.2728, 1e-4)
			So(quote.Ask, ShouldAlmostEqual, 4.4472, 1e-4)
		})
	})
}
