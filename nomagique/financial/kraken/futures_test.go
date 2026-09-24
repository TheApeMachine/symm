package kraken

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFutures(t *testing.T) {
	ctx := context.Background()

	Convey("Given Kraken Futures frames", t, func() {
		client := Futures_ServerToClient(NewFutures())
		defer client.Release()

		read := func(frame string) []map[string]any {
			So(client.Write(ctx, func(params Futures_write_Params) error {
				return params.SetData([]byte(frame))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()
			results, err := future.Struct()
			So(err, ShouldBeNil)

			records := []map[string]any{}

			if results.Which() == Records_Which_idle {
				return records
			}

			list, err := results.Records()
			So(err, ShouldBeNil)

			for index := range list.Len() {
				raw, err := list.At(index)
				So(err, ShouldBeNil)
				var record map[string]any
				So(json.Unmarshal(raw, &record), ShouldBeNil)
				records = append(records, record)
			}

			return records
		}

		Convey("A ticker is a futures_ticker record with its capture and an RFC 3339 time", func() {
			records := read(`{"feed":"ticker","product_id":"PI_XBTUSD","last":60000.5,"index":59990,"markPrice":60000.2,"openInterest":123456,"bid_size":10,"time":1690000000000,"capture":{"receivedAt":"2023-07-22T04:26:40.1Z"}}`)
			So(records, ShouldHaveLength, 1)
			So(records[0]["channel"], ShouldEqual, "futures_ticker")
			data := records[0]["data"].(map[string]any)
			So(data["mark"], ShouldEqual, 60000.2)
			So(data["bid_qty"], ShouldEqual, 10)
			So(data["timestamp"], ShouldEqual, "2023-07-22T04:26:40Z")
			So(records[0]["capture"].(map[string]any)["receivedAt"], ShouldEqual, "2023-07-22T04:26:40.1Z")
		})

		Convey("A trade keeps its fill type, and a snapshot is every trade oldest first", func() {
			trade := read(`{"feed":"trade","product_id":"PI_XBTUSD","side":"sell","type":"liquidation","qty":2,"price":60000,"time":1690000000000}`)
			So(trade[0]["channel"], ShouldEqual, "futures_trade")
			So(trade[0]["data"].(map[string]any)["type"], ShouldEqual, "liquidation")

			snapshot := read(`{"feed":"trade_snapshot","product_id":"PI_XBTUSD","trades":[{"price":2,"qty":1,"side":"buy","type":"fill","time":1690000002000},{"price":1,"qty":1,"side":"buy","type":"fill","time":1690000001000}]}`)
			So(snapshot, ShouldHaveLength, 2)
			So(snapshot[0]["data"].(map[string]any)["price"], ShouldEqual, 1)
			So(snapshot[1]["data"].(map[string]any)["price"], ShouldEqual, 2)
		})

		Convey("A subscription reply is idle, and the next frame starts clean", func() {
			So(read(`{"event":"subscribed","feed":"ticker"}`), ShouldBeEmpty)
		})

		Convey("A frame that is not JSON is an error", func() {
			So(client.Write(ctx, func(params Futures_write_Params) error {
				return params.SetData([]byte("not json"))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}
