package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

var tickerPayload = []byte(`{
  "channel":"ticker",
  "type":"snapshot",
  "data":[{
    "symbol":"CORN/USD",
    "bid":0.02015,
    "bid_qty":3500.00000,
    "ask":0.04414,
    "ask_qty":923.40000,
    "last":0.00000,
    "volume":0.00000,
    "vwap":0.00000,
    "low":0.00000,
    "high":0.00000,
    "change":0.00000,
    "change_pct":0.00,
    "trades":0,
    "timestamp":"2026-09-01T23:11:36.088011Z"
  }]
}`)

func TestNewTicker(t *testing.T) {
	Convey("Given an online quoted market with no recent trade", t, func() {
		ticker := NewTicker(tickerPayload)

		Convey("the explicit zero trade count remains distinguishable from absence", func() {
			So(ticker, ShouldNotBeNil)
			So(ticker.Data, ShouldHaveLength, 1)
			So(ticker.Data[0].Trades, ShouldNotBeNil)
			So(*ticker.Data[0].Trades, ShouldEqual, int64(0))
		})
	})
}

func BenchmarkNewTicker(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		NewTicker(tickerPayload)
	}
}
