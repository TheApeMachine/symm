package trade

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

var sampleUpdateFrame = []byte(`{
  "channel":"trade",
  "type":"update",
  "data":[
    {
      "symbol":"BTC/EUR",
      "side":"buy",
      "price":50000.1,
      "qty":0.25,
      "ord_type":"limit",
      "trade_id":4665846,
      "timestamp":"2026-05-23T02:00:00.123456789Z"
    },
    {
      "symbol":"BTC/EUR",
      "side":"sell",
      "price":50000.2,
      "qty":0.10,
      "ord_type":"market",
      "trade_id":4665847,
      "timestamp":"2026-05-23T02:00:00.223456789Z"
    }
  ]
}`)

var sampleSnapshotFrame = []byte(`{
  "channel":"trade",
  "type":"snapshot",
  "data":[
    {
      "symbol":"MATIC/USD",
      "side":"buy",
      "price":0.5147,
      "qty":6423.46326,
      "ord_type":"limit",
      "trade_id":4665846,
      "timestamp":"2023-09-25T07:48:36.925533Z"
    }
  ]
}`)

func TestSnapshotUnmarshalUpdate(t *testing.T) {
	t.Parallel()

	Convey("Given a Kraken v2 trade update frame", t, func() {
		var frame Snapshot

		err := json.Unmarshal(sampleUpdateFrame, &frame)

		Convey("It should decode every execution in data", func() {
			So(err, ShouldBeNil)
			So(frame.Channel, ShouldEqual, "trade")
			So(frame.Type, ShouldEqual, "update")
			So(frame.Data, ShouldHaveLength, 2)
			So(frame.Data[0].Price, ShouldEqual, 50000.1)
			So(frame.Data[0].Qty, ShouldEqual, 0.25)
			So(frame.Data[0].Side, ShouldEqual, "buy")
			So(frame.Data[0].OrdType, ShouldEqual, "limit")
			So(frame.Data[0].TradeID, ShouldEqual, 4665846)
			So(frame.Data[1].Side, ShouldEqual, "sell")
		})
	})
}

func TestSnapshotUnmarshalSnapshot(t *testing.T) {
	t.Parallel()

	Convey("Given a Kraken v2 trade snapshot frame", t, func() {
		var frame Snapshot

		err := json.Unmarshal(sampleSnapshotFrame, &frame)

		Convey("It should decode the snapshot payload", func() {
			So(err, ShouldBeNil)
			So(frame.Type, ShouldEqual, "snapshot")
			So(frame.Data, ShouldHaveLength, 1)
			So(frame.Data[0].Symbol, ShouldEqual, "MATIC/USD")
		})
	})
}

func TestNewSubscribe(t *testing.T) {
	t.Parallel()

	Convey("Given NewSubscribe", t, func() {
		request := NewSubscribe([]string{"BTC/EUR"})

		Convey("It should target the trade channel with snapshot", func() {
			So(request.Method, ShouldEqual, "subscribe")

			params, ok := request.Params.(Params)

			So(ok, ShouldBeTrue)
			So(params.Channel, ShouldEqual, "trade")
			So(params.Symbol, ShouldResemble, []string{"BTC/EUR"})
			So(params.Snapshot, ShouldBeTrue)
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

func BenchmarkSnapshotUnmarshalTimestamp(b *testing.B) {
	var frame Snapshot

	if err := json.Unmarshal(sampleUpdateFrame, &frame); err != nil {
		b.Fatal(err)
	}

	expected := time.Date(2026, 5, 23, 2, 0, 0, 123456789, time.UTC)

	for b.Loop() {
		if !frame.Data[0].Timestamp.Equal(expected) {
			b.Fatal("timestamp mismatch")
		}
	}
}
