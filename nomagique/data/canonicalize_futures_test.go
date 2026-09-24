package data_test

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestCanonicalizeFutures(t *testing.T) {
	Convey("Given a CanonicalizeFutures capability", t, func() {
		ctx := context.Background()
		server := data.NewCanonicalizeFutures(ctx)
		So(server, ShouldNotBeNil)

		client := data.CanonicalizeFutures_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("Raw ticker feed translates to canonical channel/data structure", func() {
			rawTicker := []byte(`{
				"feed": "ticker",
				"product_id": "PI_XBTUSD",
				"bid": 60000.0,
				"ask": 60001.0,
				"bid_size": 10.0,
				"ask_size": 5.0,
				"last": 60000.5,
				"index": 59990.0,
				"markPrice": 60000.2,
				"openInterest": 123456.0,
				"time": 1690000000000
			}`)

			err := client.Write(ctx, func(params data.CanonicalizeFutures_write_Params) error {
				return params.SetData(rawTicker)
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)

			outBytes, err := result.Out()
			So(err, ShouldBeNil)
			So(len(outBytes), ShouldBeGreaterThan, 0)

			var doc map[string]any
			So(json.Unmarshal(outBytes, &doc), ShouldBeNil)
			So(doc["channel"], ShouldEqual, "ticker")
			dataObj, ok := doc["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(dataObj["symbol"], ShouldEqual, "PI_XBTUSD")
			So(dataObj["last"], ShouldEqual, 60000.5)
			So(dataObj["bid"], ShouldEqual, 60000.0)
			So(dataObj["ask"], ShouldEqual, 60001.0)
			So(dataObj["bid_qty"], ShouldEqual, 10.0)
			So(dataObj["ask_qty"], ShouldEqual, 5.0)
			So(dataObj["index"], ShouldEqual, 59990.0)
			So(dataObj["mark"], ShouldEqual, 60000.2)
			So(dataObj["openInterest"], ShouldEqual, 123456.0)
			So(dataObj["timestamp"], ShouldEqual, 1690000000000)
		})

		Convey("Raw trade feed translates to canonical channel/data structure", func() {
			rawTrade := []byte(`{
				"feed": "trade",
				"product_id": "PI_XBTUSD",
				"side": "sell",
				"price": 59995.0,
				"qty": 1.5,
				"time": 1690000005000
			}`)

			err := client.Write(ctx, func(params data.CanonicalizeFutures_write_Params) error {
				return params.SetData(rawTrade)
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)

			outBytes, err := result.Out()
			So(err, ShouldBeNil)
			So(len(outBytes), ShouldBeGreaterThan, 0)

			var doc map[string]any
			So(json.Unmarshal(outBytes, &doc), ShouldBeNil)
			So(doc["channel"], ShouldEqual, "trade")
			dataObj, ok := doc["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(dataObj["symbol"], ShouldEqual, "PI_XBTUSD")
			So(dataObj["price"], ShouldEqual, 59995.0)
			So(dataObj["qty"], ShouldEqual, 1.5)
			So(dataObj["side"], ShouldEqual, "sell")
			So(dataObj["timestamp"], ShouldEqual, 1690000005000)
		})
	})
}
