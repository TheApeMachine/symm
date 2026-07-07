package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewTickerDataSlice(t *testing.T) {
	Convey("Given Kraken ticker data payloads", t, func() {
		payload := []byte(`[{
			"symbol": "BTC/USD",
			"bid": 43124.9,
			"bid_qty": 1.2,
			"ask": 43125.1,
			"ask_qty": 0.8,
			"last": 43125,
			"volume": 1842.5,
			"vwap": 43000.1,
			"low": 42100,
			"high": 43500,
			"change": 125,
			"change_pct": 0.29,
			"timestamp": "2023-10-06T17:35:55.440295Z"
		}]`)

		tickers := NewTickerDataSlice(payload)

		Convey("It should decode level one market fields", func() {
			So(len(tickers), ShouldEqual, 1)

			ticker := tickers[0]

			So(ticker.Symbol, ShouldEqual, "BTC/USD")
			So(ticker.Bid.Float64(), ShouldAlmostEqual, 43124.9)
			So(ticker.AskQty, ShouldAlmostEqual, 0.8)
			So(ticker.ChangePct, ShouldAlmostEqual, 0.29)
			So(ticker.Timestamp.IsZero(), ShouldBeFalse)
		})
	})
}
