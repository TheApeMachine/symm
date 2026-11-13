package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
)

func TestDecodeLevel3(t *testing.T) {
	Convey("Setup", t, func() {
		raw := []byte(`{
			"channel":"level3",
			"type":"update",
			"data":[{
				"symbol":"MATIC/USD",
				"checksum":1,
				"bids":[],
				"asks":[{
					"event":"delete",
					"order_id":"OOIATY-6EIWY-ACVIUN",
					"limit_price":0.5636,
					"order_qty":302.89736033,
					"timestamp":"2023-10-06T18:21:00.097010033Z"
				}]
			}]
		}`)

		Convey("When DecodeLevel3 parses a delete event", func() {
			decoded := DecodeLevel3(raw)

			Convey("event and order qty decode on the wire", func() {
				So(decoded, ShouldNotBeNil)
				So(decoded.Channel, ShouldEqual, "level3")
				So(decoded.Data[0].Asks[0].Event, ShouldEqual, "delete")
				So(decoded.Data[0].Asks[0].OrderQty.String(), ShouldEqual, "302.89736033")
			})

			Convey("ToMarket keeps the delete event on the book", func() {
				out := decoded.ToMarket(market.NewLevel3()).(*market.Level3)
				So(out.Data[0].Asks[0].Event, ShouldEqual, "delete")
				So(out.Data[0].Asks[0].OrderQty.String(), ShouldEqual, "302.89736033")
			})
		})
	})
}
