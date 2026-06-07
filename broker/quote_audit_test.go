package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestQuoteAuditFields(t *testing.T) {
	Convey("Given a fresh quote and order size", t, func() {
		now := time.Unix(1_700_000_000, 0).UTC()
		quote := Quote{
			Symbol:    "ALT/EUR",
			Bid:       99,
			Ask:       101,
			Last:      100,
			UpdatedAt: now.Add(-2 * time.Second),
			Book: market.Book{
				Asks: []market.BookLevel{{Price: 101, Qty: 10}},
			},
		}

		fields := QuoteAuditFields(quote, trading.Buy, 1, now)

		Convey("It should expose spread, age, and depth coverage", func() {
			So(fields["spread_bps"], ShouldBeGreaterThan, 0)
			So(fields["quote_age_ms"], ShouldBeGreaterThan, 0)
			So(fields["depth_coverage"], ShouldEqual, 1)
		})
	})
}
