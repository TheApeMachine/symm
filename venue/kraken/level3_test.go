package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
)

func TestDecodeLevel3DeleteEvent(t *testing.T) {
	Convey("Update frames keep event through ToMarket", t, func() {
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

		decoded := DecodeLevel3(raw)
		So(decoded, ShouldNotBeNil)
		So(decoded.Data[0].Asks[0].Event, ShouldEqual, "delete")

		out := decoded.ToMarket(market.NewLevel3()).(*market.Level3)
		So(out.Data[0].Asks[0].Event, ShouldEqual, "delete")
		So(out.Data[0].Asks[0].OrderQty.String(), ShouldEqual, "302.89736033")
	})
}
