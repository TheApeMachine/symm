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

	Convey("Given a Kraken ticker channel envelope", t, func() {
		raw := []byte(`{"channel":"ticker","type":"snapshot","data":[{"symbol":"ALGO/USD","bid":0.10025,"bid_qty":740.0,"ask":0.10036,"ask_qty":1361.44813783,"last":0.10035,"volume":997038.98383185,"vwap":0.10148,"low":0.09979,"high":0.10285,"change":-0.00017,"change_pct":-0.17,"timestamp":"2023-09-25T09:04:31.742648Z"}]}`)

		tickers := NewTickerDataSlice(raw)

		Convey("It should decode level one market fields from the envelope", func() {
			So(len(tickers), ShouldEqual, 1)

			ticker := tickers[0]

			So(ticker.Symbol, ShouldEqual, "ALGO/USD")
			So(ticker.Last.Float64(), ShouldAlmostEqual, 0.10035)
			So(ticker.Bid.Float64(), ShouldAlmostEqual, 0.10025)
			So(ticker.Ask.Float64(), ShouldAlmostEqual, 0.10036)
			So(ticker.Volume, ShouldAlmostEqual, 997038.98383185)
		})
	})
}
