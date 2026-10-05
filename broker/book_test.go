package broker_test

import (
	"context"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestBook_SetMutationsAndApplyMeasurement(t *testing.T) {
	Convey("Given a broker.Book", t, func() {
		ctx := context.Background()
		normalizer := spot.NewNormalizer()
		book := broker.NewBook(ctx, normalizer)

		Convey("SetMutations captures accepted level3 frames", func() {
			var captured []kraken.Level3Data
			book.SetMutations(func(data []kraken.Level3Data) {
				captured = append(captured, data...)
			})

			bidPrice := decimal.NewFromFloat64(50000.0)
			bidQty := decimal.NewFromFloat64(1.5)
			askPrice := decimal.NewFromFloat64(50005.0)
			askQty := decimal.NewFromFloat64(2.0)

			payload := &kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{
					{
						Symbol: "BTC/USD",
						Bids: []kraken.Level3Order{
							{
								OrderID:    "bid-1",
								LimitPrice: bidPrice,
								OrderQty:   bidQty,
								Timestamp:  time.Now().UTC(),
								Event:      "add",
							},
						},
						Asks: []kraken.Level3Order{
							{
								OrderID:    "ask-1",
								LimitPrice: askPrice,
								OrderQty:   askQty,
								Timestamp:  time.Now().UTC(),
								Event:      "add",
							},
						},
						Timestamp: time.Now().UTC(),
					},
				},
			}

			err := book.Update(payload)
			So(err, ShouldBeNil)
			So(len(captured), ShouldEqual, 1)
			So(captured[0].Symbol, ShouldEqual, "BTC/USD")
			So(len(captured[0].Bids), ShouldEqual, 1)
			So(captured[0].Bids[0].OrderID, ShouldEqual, "bid-1")
			So(len(captured[0].Asks), ShouldEqual, 1)
			So(captured[0].Asks[0].OrderID, ShouldEqual, "ask-1")
		})

		Convey("ApplyMeasurement reconstructs an isolated book chronologically", func() {
			isolatedBook := broker.NewBook(ctx, normalizer)

			now := time.Now().UTC()
			bidPrice := decimal.NewFromFloat64(45000.0)
			bidQty := decimal.NewFromFloat64(3.0)

			bidMeasurement := data.NewMeasurement("websocket", map[string]data.Metric{
				"limit_price": {Raw: bidPrice.Float64(), Exact: bidPrice},
				"order_qty":   {Raw: bidQty.Float64(), Exact: bidQty},
			})
			bidMeasurement.Label = "BTC/USD"
			bidMeasurement.At = now
			bidMeasurement.SetMetadata("type", "snapshot")
			bidMeasurement.SetMetadata("order_id", "order-b1")
			bidMeasurement.SetMetadata("side", "bid")
			bidMeasurement.SetMetadata("event", "add")
			bidMeasurement.SetProvenance("ingress_channel", "level3")

			err := isolatedBook.ApplyMeasurement(bidMeasurement)
			So(err, ShouldBeNil)

			askPrice := decimal.NewFromFloat64(45010.0)
			askQty := decimal.NewFromFloat64(1.0)

			askMeasurement := data.NewMeasurement("websocket", map[string]data.Metric{
				"limit_price": {Raw: askPrice.Float64(), Exact: askPrice},
				"order_qty":   {Raw: askQty.Float64(), Exact: askQty},
			})
			askMeasurement.Label = "BTC/USD"
			askMeasurement.At = now.Add(time.Millisecond)
			askMeasurement.SetMetadata("type", "update")
			askMeasurement.SetMetadata("order_id", "order-a1")
			askMeasurement.SetMetadata("side", "ask")
			askMeasurement.SetMetadata("event", "add")
			askMeasurement.SetProvenance("ingress_channel", "level3")

			err = isolatedBook.ApplyMeasurement(askMeasurement)
			So(err, ShouldBeNil)

			var verifiedBestBid *decimal.Decimal
			var verifiedBestAsk *decimal.Decimal

			isolatedBook.Book("BTC/USD", func(b *spotbook.Book) {
				if b != nil && b.BestBid() != nil {
					verifiedBestBid = b.BestBid().Price
				}
				if b != nil && b.BestAsk() != nil {
					verifiedBestAsk = b.BestAsk().Price
				}
			})

			So(verifiedBestBid, ShouldNotBeNil)
			So(verifiedBestBid.Cmp(bidPrice), ShouldEqual, 0)
			So(verifiedBestAsk, ShouldNotBeNil)
			So(verifiedBestAsk.Cmp(askPrice), ShouldEqual, 0)
		})
	})
}
