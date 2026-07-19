package websocket

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	. "github.com/smartystreets/goconvey/convey"
)

/*
TestSubscribeLevel3EnsuresBooks proves books exist before SubL3 returns so the
first inbound snapshot cannot race an empty library.
*/
func TestSubscribeLevel3EnsuresBooks(t *testing.T) {
	Convey("Given an L3 Live transport", t, func() {
		live := New(context.Background(), nil, true, Level3WebSocketURL)
		live.client.Reconnect = func() {}

		Convey("When SubscribeLevel3 is called for a batch", func() {
			// SubL3 needs a connected client; create books is the contract under test.
			live.ensureLevel3Books([]string{"SYND/USD", "HMSTR/USD"}, 10)

			Convey("Then each symbol is present in the BookManager", func() {
				So(live.books.GetBook("SYND/USD"), ShouldNotBeNil)
				So(live.books.GetBook("HMSTR/USD"), ShouldNotBeNil)
				So(live.books.GetBook("SYND/USD").EnableMaxDepth, ShouldBeFalse)
			})
		})
	})
}

/*
TestIngestLevel3SentCreatesBooksFromSubscribe mirrors Kraken's OnSent path:
the outbound subscribe request must CreateBook before snapshots arrive.
*/
func TestIngestLevel3SentCreatesBooksFromSubscribe(t *testing.T) {
	Convey("Given an L3 Live with an empty library", t, func() {
		live := New(context.Background(), nil, true, Level3WebSocketURL)
		live.client.Reconnect = func() {}

		raw := []byte(`{
			"method":"subscribe",
			"params":{
				"channel":"level3",
				"symbol":["LINK/USD","BIO/USD"],
				"depth":10
			}
		}`)

		Convey("When the outbound subscribe is ingested", func() {
			live.ingestLevel3Sent(&callback.Event[*sdkkraken.WebSocketMessage]{
				Data: sdkkraken.NewWebSocketMessage(raw),
			})

			Convey("Then BookManager owns both symbols", func() {
				So(live.books.GetBook("LINK/USD"), ShouldNotBeNil)
				So(live.books.GetBook("BIO/USD"), ShouldNotBeNil)
			})
		})
	})
}

/*
TestUpdateLevel3RecoversDeleteOnEmptyBook proves the SDK nil-Level panic on
delete-against-missing-price is recovered so drainLevel3 cannot crash the process.
*/
func TestUpdateLevel3RecoversDeleteOnEmptyBook(t *testing.T) {
	Convey("Given an empty SDK book (post-invalidate / pre-snapshot)", t, func() {
		live := New(context.Background(), nil, true, Level3WebSocketURL)
		live.client.Reconnect = func() {}
		live.books.CreateBook("SYND/USD", 10)

		// Numeric limit_price (json.Number) reaches Side.update; delete on a
		// missing level is the SDK nil-receiver panic we must recover.
		raw := []byte(`{
			"channel":"level3",
			"type":"update",
			"data":[{
				"symbol":"SYND/USD",
				"checksum":1,
				"bids":[{
					"event":"delete",
					"order_id":"ghost",
					"limit_price":0.01,
					"timestamp":"2026-07-19T10:00:00Z"
				}],
				"asks":[]
			}]
		}`)

		Convey("When a delete update hits a price level that was never inserted", func() {
			err := live.updateLevel3(&callback.Event[*sdkkraken.WebSocketMessage]{
				Data: sdkkraken.NewWebSocketMessage(raw),
			})

			Convey("Then the panic is returned as an error instead of crashing", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "level3 apply panic")
			})
		})
	})
}
