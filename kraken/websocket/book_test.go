package websocket

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdk "github.com/krakenfx/api-go/v2/pkg/kraken"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestBookUpdate(t *testing.T) {
	Convey("Given a managed level3 book", t, func() {
		managed := NewBook(context.Background())
		managed.manager.CreateBook("BTC/USD", 10)

		Convey("It should ignore delete updates for missing levels without recreating state", func() {
			raw := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD","bids":[{"event":"delete","order_id":"missing-order","limit_price":65432.1,"timestamp":"2026-08-02T12:00:00Z"}],"asks":[],"checksum":0}]}`)
			payload := kraken.NewLevel3(raw)
			event := &callback.Event[*sdk.WebSocketMessage]{
				Data: sdk.NewWebSocketMessage(raw),
			}
			var err error

			So(func() {
				err = managed.Update(event, payload)
			}, ShouldNotPanic)
			So(err, ShouldBeNil)
			So(managed.Get("BTC/USD"), ShouldNotBeNil)
			So(len(managed.Get("BTC/USD").Bids.Levels), ShouldEqual, 0)
			So(len(managed.Get("BTC/USD").Asks.Levels), ShouldEqual, 0)
		})

		Convey("It should normalize omitted side arrays before forwarding to the book manager", func() {
			raw := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD","checksum":0,"bids":[]}]}`)
			payload := kraken.NewLevel3(raw)
			event := &callback.Event[*sdk.WebSocketMessage]{
				Data: sdk.NewWebSocketMessage(raw),
			}
			var err error

			So(func() {
				err = managed.Update(event, payload)
			}, ShouldNotPanic)
			So(err, ShouldBeNil)
		})

		Convey("It should not panic when a bid price key exists with a nil level pointer", func() {
			book := managed.Get("BTC/USD")
			book.Bids.Levels["65432.1"] = nil

			raw := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD","bids":[{"order_id":"order-1","limit_price":65432.1,"order_qty":1.25,"timestamp":"2026-08-02T12:00:00Z"}],"asks":[],"checksum":0}]}`)
			payload := kraken.NewLevel3(raw)
			event := &callback.Event[*sdk.WebSocketMessage]{
				Data: sdk.NewWebSocketMessage(raw),
			}
			var err error

			So(func() {
				err = managed.Update(event, payload)
			}, ShouldNotPanic)
			So(err, ShouldBeNil)
			So(managed.Get("BTC/USD").Bids.Levels["65432.1"], ShouldNotBeNil)
		})

		Convey("It should not panic when an ask price key exists with a nil level pointer", func() {
			book := managed.Get("BTC/USD")
			book.Asks.Levels["65433.2"] = nil

			raw := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD","bids":[],"asks":[{"order_id":"order-2","limit_price":65433.2,"order_qty":0.75,"timestamp":"2026-08-02T12:00:00Z"}],"checksum":0}]}`)
			payload := kraken.NewLevel3(raw)
			event := &callback.Event[*sdk.WebSocketMessage]{
				Data: sdk.NewWebSocketMessage(raw),
			}
			var err error

			So(func() {
				err = managed.Update(event, payload)
			}, ShouldNotPanic)
			So(err, ShouldBeNil)
			So(managed.Get("BTC/USD").Asks.Levels["65433.2"], ShouldNotBeNil)
		})

		Convey("It should not panic when a nil level pointer exists at one price and a later row updates another price", func() {
			book := managed.Get("BTC/USD")
			book.Bids.Levels["65432.1"] = nil

			raw := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD","bids":[{"order_id":"order-a","limit_price":65431.9,"order_qty":1.00,"timestamp":"2026-08-02T12:00:00Z"},{"order_id":"order-b","limit_price":65432.1,"order_qty":2.00,"timestamp":"2026-08-02T12:00:01Z"}],"asks":[],"checksum":0}]}`)
			payload := kraken.NewLevel3(raw)
			event := &callback.Event[*sdk.WebSocketMessage]{
				Data: sdk.NewWebSocketMessage(raw),
			}
			var err error

			So(func() {
				err = managed.Update(event, payload)
			}, ShouldNotPanic)
			So(err, ShouldBeNil)
			So(managed.Get("BTC/USD").Bids.Levels["65431.9"], ShouldNotBeNil)
			So(managed.Get("BTC/USD").Bids.Levels["65432.1"], ShouldNotBeNil)
		})

		Convey("It should not panic when a price is inserted and later reduced to zero in the same payload", func() {
			raw := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD","bids":[{"order_id":"order-c","limit_price":65430.5,"order_qty":1.50,"timestamp":"2026-08-02T12:00:00Z"},{"order_id":"order-c","limit_price":65430.5,"order_qty":0.0,"timestamp":"2026-08-02T12:00:01Z"}],"asks":[],"checksum":0}]}`)
			payload := kraken.NewLevel3(raw)
			event := &callback.Event[*sdk.WebSocketMessage]{
				Data: sdk.NewWebSocketMessage(raw),
			}
			var err error

			So(func() {
				err = managed.Update(event, payload)
			}, ShouldNotPanic)
			So(err, ShouldBeNil)
		})

		Convey("It should ignore a repeated removal after an earlier row removes the level in the same payload", func() {
			raw := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD","bids":[{"order_id":"order-d","limit_price":65429.5,"order_qty":1.50,"timestamp":"2026-08-02T12:00:00Z"},{"event":"delete","order_id":"order-d","limit_price":65429.5,"timestamp":"2026-08-02T12:00:01Z"},{"event":"delete","order_id":"order-d","limit_price":65429.5,"timestamp":"2026-08-02T12:00:02Z"}],"asks":[],"checksum":0}]}`)
			payload := kraken.NewLevel3(raw)
			event := &callback.Event[*sdk.WebSocketMessage]{
				Data: sdk.NewWebSocketMessage(raw),
			}
			var err error

			So(func() {
				err = managed.Update(event, payload)
			}, ShouldNotPanic)
			So(err, ShouldBeNil)
			So(managed.Get("BTC/USD").Bids.Levels["65429.5"], ShouldBeNil)
			So(len(payload.Data[0].Bids), ShouldEqual, 2)
		})

		Convey("It should handle level3 frames with missing data payload", func() {
			raw := []byte(`{"channel":"level3","type":"update"}`)
			payload := kraken.NewLevel3(raw)
			event := &callback.Event[*sdk.WebSocketMessage]{
				Data: sdk.NewWebSocketMessage(raw),
			}
			var err error

			So(func() {
				err = managed.Update(event, payload)
			}, ShouldNotPanic)
			So(err, ShouldBeNil)
		})
	})
}

func BenchmarkBookUpdate(b *testing.B) {
	managed := NewBook(context.Background())
	managed.manager.CreateBook("BTC/USD", 10)
	raw := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD","bids":[{"event":"delete","order_id":"missing-order","limit_price":65432.1,"timestamp":"2026-08-02T12:00:00Z"}],"asks":[],"checksum":0}]}`)
	payload := kraken.NewLevel3(raw)
	event := &callback.Event[*sdk.WebSocketMessage]{
		Data: sdk.NewWebSocketMessage(raw),
	}

	for b.Loop() {
		_ = managed.Update(event, payload)
	}
}
