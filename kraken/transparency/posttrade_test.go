package transparency

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

const postTradeFixture = `{
	"error": [],
	"result": {
		"last_ts": "2026-05-29T15:34:03.712559166Z",
		"count": 1,
		"trades": [
			{
				"trade_id": "OUTNLW-YR5DF-JVWGII",
				"price": "73930.40000",
				"quantity": "0.00096379",
				"symbol": "BTC/USD",
				"description": "Bitcoin / US Dollar",
				"base_asset": "XBT",
				"base_notation": "UNIT",
				"base_dti_code": "V15WLZJMF",
				"base_dti_short_name": "BTC,XBT",
				"quote_asset": "USD",
				"quote_notation": "MONE",
				"quote_dti_code": "",
				"quote_dti_short_name": "",
				"trade_venue": "PGSL",
				"trade_ts": "2026-05-29T15:34:03.712559166Z",
				"publication_venue": "PGSL",
				"publication_ts": "2026-05-29T15:34:03.712559166Z"
			}
		]
	}
}`

func TestNewPostTrade(t *testing.T) {
	convey.Convey("Given a Kraken post-trade payload", t, func() {
		posttrade := &PostTrade{}

		convey.Convey("It should unmarshal trade transparency fields", func() {
			convey.So(json.Unmarshal([]byte(postTradeFixture), &public.Response{
				Result: posttrade,
			}), convey.ShouldBeNil)
			convey.So(posttrade.Count, convey.ShouldEqual, 1)
			convey.So(len(posttrade.Trades), convey.ShouldEqual, 1)
			convey.So(posttrade.Trades[0].TradeID, convey.ShouldEqual, "OUTNLW-YR5DF-JVWGII")
			convey.So(posttrade.Trades[0].Symbol, convey.ShouldEqual, "BTC/USD")
			convey.So(posttrade.Trades[0].TradeVenue, convey.ShouldEqual, "PGSL")
			convey.So(
				posttrade.LastTs,
				convey.ShouldEqual,
				time.Date(2026, 5, 29, 15, 34, 3, 712559166, time.UTC),
			)
		})
	})
}

func BenchmarkNewPostTrade(b *testing.B) {
	payload := []byte(postTradeFixture)

	for b.Loop() {
		posttrade := &PostTrade{}
		_ = json.Unmarshal(payload, &public.Response{Result: posttrade})
	}
}
