package ohlc

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

var sampleUpdateFrame = []byte(`{
  "channel":"ohlc",
  "type":"update",
  "data":[
    {
      "symbol":"BTC/EUR",
      "open":50000.0,
      "high":50100.0,
      "low":49900.0,
      "close":50050.0,
      "vwap":50025.0,
      "volume":12.5,
      "interval_begin":"2026-05-23T02:00:00.123456789Z",
      "interval":1
    }
  ]
}`)

func TestSnapshotUnmarshalUpdate(t *testing.T) {
	t.Parallel()

	Convey("Given a Kraken v2 OHLC update frame", t, func() {
		var frame Snapshot

		err := json.Unmarshal(sampleUpdateFrame, &frame)

		Convey("It should decode every candle in data", func() {
			So(err, ShouldBeNil)
			So(frame.Channel, ShouldEqual, "ohlc")
			So(frame.Type, ShouldEqual, "update")
			So(frame.Data, ShouldHaveLength, 1)
			So(frame.Data[0].Symbol, ShouldEqual, "BTC/EUR")
			So(frame.Data[0].Close, ShouldEqual, 50050.0)
			So(frame.Data[0].Interval, ShouldEqual, 1)
		})
	})
}

func TestNewSubscribe(t *testing.T) {
	t.Parallel()

	Convey("Given NewSubscribe", t, func() {
		request := NewSubscribe([]string{"BTC/EUR"})

		Convey("It should target the ohlc channel with snapshot", func() {
			So(request.Method, ShouldEqual, "subscribe")

			params, ok := request.Params.(Params)

			So(ok, ShouldBeTrue)
			So(params.Channel, ShouldEqual, "ohlc")
			So(params.Symbol, ShouldResemble, []string{"BTC/EUR"})
			So(params.Snapshot, ShouldBeTrue)
			So(params.Interval, ShouldEqual, 1)
		})
	})
}

func BenchmarkSnapshotUnmarshal(b *testing.B) {
	for b.Loop() {
		var frame Snapshot

		if err := json.Unmarshal(sampleUpdateFrame, &frame); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSnapshotUnmarshalIntervalBegin(b *testing.B) {
	var frame Snapshot

	if err := json.Unmarshal(sampleUpdateFrame, &frame); err != nil {
		b.Fatal(err)
	}

	expected := time.Date(2026, 5, 23, 2, 0, 0, 123456789, time.UTC)

	for b.Loop() {
		if !frame.Data[0].IntervalBegin.Equal(expected) {
			b.Fatal("interval_begin mismatch")
		}
	}
}
