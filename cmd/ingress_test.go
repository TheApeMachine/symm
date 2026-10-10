package cmd

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
)

func TestSubscribeRejection(t *testing.T) {
	Convey("Given venue method acknowledgements", t, func() {
		Convey("a rejected subscribe is an error", func() {
			err := subscribeRejection([]byte(`{"method":"subscribe","success":false,"error":"Token(s) not found","result":{"channel":"level3","symbol":"BTC/USD"}}`))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Token(s) not found")
			So(err.Error(), ShouldContainSubstring, "level3")
		})

		Convey("a level3 snapshot rate-limit rejection (venue frame, top-level channel) is an error", func() {
			err := subscribeRejection([]byte(`{"channel":"level3","error":"Rate limit for snapshot requests exceeded, when trying to subscribe to YFI/USD level3 snapshot","method":"subscribe","status":"error","success":false,"time_in":"2026-10-10T06:06:44.517923Z","time_out":"2026-10-10T06:06:44.525929Z"}`))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Rate limit for snapshot requests exceeded")
			So(err.Error(), ShouldContainSubstring, `channel="level3"`)
		})

		Convey("an accepted subscribe is not", func() {
			So(subscribeRejection([]byte(`{"method":"subscribe","success":true,"result":{"channel":"trade","symbol":"BTC/USD"}}`)), ShouldBeNil)
		})

		Convey("a rejected order ack is left to the order path", func() {
			So(subscribeRejection([]byte(`{"method":"add_order","success":false,"error":"EOrder:Insufficient funds"}`)), ShouldBeNil)
		})

		Convey("a subscribe ack without success is not treated as a rejection", func() {
			So(subscribeRejection([]byte(`{"method":"subscribe"}`)), ShouldBeNil)
		})

		Convey("non-JSON and pong frames are ignored", func() {
			So(subscribeRejection([]byte(`not json`)), ShouldBeNil)
			So(subscribeRejection([]byte(`{"method":"pong"}`)), ShouldBeNil)
		})
	})
}

func TestHandleTrade(t *testing.T) {
	Convey("Given incoming trade frames", t, func() {
		ctx := context.Background()
		normalizer := spot.NewNormalizer()
		book := broker.NewBook(ctx, normalizer)
		price := broker.NewPrice(ctx, book, broker.NewPaper(ctx), nil, normalizer)
		storeTee := hindsight.NewStoreTee(ctx, "testStoreTee")
		storeTee.Transition(nmruntime.READY)

		Convey("a valid trade frame produces a trade measurement and invokes afterTrade", func() {
			tradeJSON := []byte(`{
				"channel": "trade",
				"type": "update",
				"data": [
					{
						"symbol": "BTC/USD",
						"side": "buy",
						"price": "65000.50",
						"qty": 0.5,
						"ord_type": "limit",
						"trade_id": 1234567,
						"timestamp": "2026-10-07T14:00:00.000000Z"
					}
				]
			}`)

			invoked := false
			err := handleTrade(tradeJSON, 100, price, nil, storeTee, "public", func() {
				invoked = true
			})

			So(err, ShouldBeNil)
			So(invoked, ShouldBeTrue)
			So(storeTee.Pending(), ShouldEqual, 1)

			measurement := storeTee.Pop()
			So(measurement, ShouldNotBeNil)
			So(measurement.Source, ShouldEqual, "spot:trade")
			So(measurement.Label, ShouldEqual, "BTC/USD")
		})

		Convey("an unprocessable trade frame returns an error", func() {
			err := handleTrade([]byte(`invalid json`), 100, price, nil, storeTee, "public", nil)
			So(err, ShouldNotBeNil)
		})

		Convey("a trade missing timestamp returns a validation error", func() {
			badTrade := []byte(`{
				"channel": "trade",
				"type": "update",
				"data": [
					{
						"symbol": "BTC/USD",
						"side": "buy",
						"price": "65000.50",
						"qty": 0.5,
						"ord_type": "limit",
						"trade_id": 1234568
					}
				]
			}`)
			err := handleTrade(badTrade, 100, price, nil, storeTee, "public", nil)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestHandleLevel3(t *testing.T) {
	Convey("Given incoming level3 frames", t, func() {
		ctx := context.Background()
		normalizer := spot.NewNormalizer()
		book := broker.NewBook(ctx, normalizer)
		storeTee := hindsight.NewStoreTee(ctx, "testStoreTee")
		storeTee.Transition(nmruntime.READY)

		Convey("a valid level3 frame updates book and pushes order measurements", func() {
			level3JSON := []byte(`{
				"channel": "level3",
				"type": "snapshot",
				"data": [
					{
						"symbol": "BTC/USD",
						"type": "snapshot",
						"timestamp": "2026-10-07T14:00:00.000000Z",
						"checksum": 3427670378,
						"bids": [
							{
								"order_id": "O1",
								"limit_price": 64999.0,
								"order_qty": 1.5,
								"timestamp": "2026-10-07T14:00:00.000000Z"
							}
						],
						"asks": []
					}
				]
			}`)

			err := handleLevel3(level3JSON, 100, book, storeTee, "public")
			So(err, ShouldBeNil)
			So(storeTee.Pending(), ShouldEqual, 1)

			measurement := storeTee.Pop()
			So(measurement, ShouldNotBeNil)
			So(measurement.Source, ShouldEqual, "spot:level3")
			So(measurement.Label, ShouldEqual, "BTC/USD")
		})

		Convey("a level3 frame missing timestamp returns an error", func() {
			badLevel3 := []byte(`{
				"channel": "level3",
				"type": "snapshot",
				"data": [
					{
						"symbol": "BTC/USD",
						"type": "snapshot",
						"checksum": 12345,
						"bids": [
							{
								"order_id": "O1",
								"limit_price": 64999.0,
								"order_qty": 1.5
							}
						],
						"asks": []
					}
				]
			}`)

			err := handleLevel3(badLevel3, 100, book, storeTee, "public")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestTeeOf(t *testing.T) {
	Convey("Given capture disabled, so no ingress StoreTee exists", t, func() {
		ctx := context.Background()
		normalizer := spot.NewNormalizer()
		book := broker.NewBook(ctx, normalizer)
		price := broker.NewPrice(ctx, book, broker.NewPaper(ctx), nil, normalizer)
		trade := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy",` +
			`"price":"65000.50","qty":0.5,"ord_type":"limit","trade_id":1,"timestamp":"2026-10-07T14:00:00.000000Z"}]}`)
		level3 := []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"BTC/USD","type":"snapshot",` +
			`"timestamp":"2026-10-07T14:00:00.000000Z","checksum":3427670378,"bids":[{"order_id":"O1",` +
			`"limit_price":64999.0,"order_qty":1.5,"timestamp":"2026-10-07T14:00:00.000000Z"}],"asks":[]}]}`)

		var missing *hindsight.StoreTee

		Convey("teeOf returns a true nil interface", func() {
			So(teeOf(missing) == nil, ShouldBeTrue)
			So(teeOf(hindsight.NewStoreTee(ctx, "present")), ShouldNotBeNil)
		})

		Convey("the ingress handlers skip it instead of pushing into a nil tee", func() {
			So(func() { _ = handleTrade(trade, 100, price, nil, teeOf(missing), "public", nil) }, ShouldNotPanic)
			So(func() { _ = handleLevel3(level3, 100, book, teeOf(missing), "public") }, ShouldNotPanic)
		})

		Convey("passing the nil pointer directly is the crash teeOf prevents", func() {
			So(func() { _ = handleTrade(trade, 101, price, nil, missing, "public", nil) }, ShouldPanic)
		})
	})
}
