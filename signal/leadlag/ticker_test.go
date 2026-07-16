package leadlag

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTickerOn(testingTB *testing.T) {
	Convey("Given a leadlag ticker ingestor", testingTB, func() {
		ticker := &Ticker{cache: tickerCache()}
		payload := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"ALGO/USD","bid":0.10025,"bid_qty":740,"ask":0.10035,"ask_qty":740,"last":0.10035,"volume":997038.98,"vwap":0.10148,"low":0.09979,"high":0.10285,"change_pct":-0.17,"timestamp":"2023-09-25T09:04:31.742648Z"}]}`)

		Convey("When a ticker frame arrives", func() {
			ticker.On(payload)

			Convey("Then ticker rows should accumulate in cache", func() {
				So(len(tickerRows(ticker.cache)), ShouldEqual, 1)
				So(tickerRows(ticker.cache)[0].Symbol, ShouldEqual, "ALGO/USD")
			})
		})
	})
}
