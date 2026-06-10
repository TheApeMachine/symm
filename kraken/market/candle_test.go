package market

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/types"
)

func TestNewCandleParams(t *testing.T) {
	Convey("Given symbols and interval", t, func() {
		raw, err := NewCandleParams([]string{"BTC/USD"}, 1)

		Convey("It should marshal subscribe params", func() {
			So(err, ShouldBeNil)
			So(raw, ShouldNotBeNil)
			So(string(raw), ShouldContainSubstring, `"channel":"ohlc"`)
		})
	})
}

func TestCandleUpdatesUnmarshal(t *testing.T) {
	Convey("Given a Kraken ohlc batch frame", t, func() {
		message := types.NewSocketMessage()
		message.Data = json.RawMessage(`[{
			"symbol":"BTC/USD",
			"open":61020.5,
			"high":61100,
			"low":60950,
			"close":61080,
			"volume":12.5,
			"interval_begin":"2024-06-01T12:34:56Z",
			"interval":1
		}]`)

		var updates CandleUpdates

		Convey("It should decode the batch", func() {
			err := updates.Unmarshal(message)

			So(err, ShouldBeNil)
			So(len(updates), ShouldEqual, 1)
			So(updates[0].Symbol, ShouldEqual, "BTC/USD")
			So(updates[0].Open, ShouldEqual, 61020.5)
		})
	})
}

func TestCandleUpdateIntervalSec(t *testing.T) {
	Convey("Given a candle with RFC3339 interval_begin", t, func() {
		candle := &CandleUpdate{
			Symbol:        "BTC/USD",
			IntervalBegin: "2024-06-01T12:34:56Z",
			Open:          100,
			High:          110,
			Low:           90,
			Close:         105,
			Volume:        12.5,
		}

		Convey("It should parse unix seconds", func() {
			expected, parseErr := time.Parse(time.RFC3339, candle.IntervalBegin)
			So(parseErr, ShouldBeNil)

			sec, err := candle.IntervalSec()

			So(err, ShouldBeNil)
			So(sec, ShouldEqual, expected.Unix())
		})

		Convey("It should emit a trade-chart wire frame", func() {
			expected, parseErr := time.Parse(time.RFC3339, candle.IntervalBegin)
			So(parseErr, ShouldBeNil)

			frame, err := candle.UIFrame()

			So(err, ShouldBeNil)
			So(frame["symbol"], ShouldEqual, "BTC/USD")
			So(frame["sec"], ShouldEqual, expected.Unix())
			So(frame["close"], ShouldEqual, 105)
		})
	})

	Convey("Given an empty interval_begin", t, func() {
		candle := &CandleUpdate{Symbol: "BTC/USD"}

		Convey("It should return an error", func() {
			_, err := candle.IntervalSec()

			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkCandleUpdateUIFrame(b *testing.B) {
	candle := &CandleUpdate{
		Symbol:        "BTC/USD",
		IntervalBegin: "2024-06-01T12:34:56Z",
		Open:          100,
		High:          110,
		Low:           90,
		Close:         105,
		Volume:        12.5,
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = candle.UIFrame()
	}
}
