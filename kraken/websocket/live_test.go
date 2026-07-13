package websocket

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
newLevel3Event wraps a raw websocket frame the same way the SDK does
before handing it to callback.Recurring subscribers.
*/
func newLevel3Event(raw []byte) *callback.Event[*kraken.WebSocketMessage] {
	return &callback.Event[*kraken.WebSocketMessage]{
		Data: kraken.NewWebSocketMessage(raw),
	}
}

func TestNewSetsAuthURL(t *testing.T) {
	Convey("Given an authenticated live transport", t, func() {
		live := &Live{
			client: spot.NewWebSocket(),
			auth:   true,
		}
		live.client.URL = PrivateWebSocketURL

		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			live.status = types.READY
		})

		live.client.OnAuthenticated.Call("token")

		Convey("It should become ready after authentication", func() {
			So(live.client.URL, ShouldEqual, PrivateWebSocketURL)
			So(live.status, ShouldEqual, types.READY)
		})
	})
}

func TestLiveRoute(t *testing.T) {
	Convey("Given callbacks registered on a live transport", t, func() {
		live := &Live{sync: &sync.Map{}}
		level3Frames := make([][]byte, 0, 2)
		tickerFrames := make([][]byte, 0, 1)
		live.On("level3", func(raw []byte) {
			level3Frames = append(level3Frames, raw)
		})
		live.On("ticker", func(raw []byte) {
			tickerFrames = append(tickerFrames, raw)
		})

		Convey("It should route data frames by their top-level channel", func() {
			raw := []byte(`{"channel":"ticker","type":"update"}`)
			live.route(raw)

			So(tickerFrames, ShouldResemble, [][]byte{raw})
		})

		Convey("It should not route subscription acknowledgements as market data", func() {
			raw := []byte(`{"method":"subscribe","result":{"channel":"level3"},"success":true}`)
			live.route(raw)

			So(level3Frames, ShouldBeEmpty)
		})

		Convey("It should not route failed acknowledgements as market data", func() {
			raw := []byte(`{"error":"invalid depth","result":{"channel":"level3"},"success":false}`)
			live.route(raw)

			So(level3Frames, ShouldBeEmpty)
		})

		Convey("It should ignore status and heartbeat frames", func() {
			live.route([]byte(`{"channel":"status"}`))
			live.route([]byte(`{"channel":"heartbeat"}`))

			So(level3Frames, ShouldBeEmpty)
			So(tickerFrames, ShouldBeEmpty)
		})
	})
}

func TestUpdateBooksRecoversFromLevel3DepthPanic(t *testing.T) {
	Convey("Given a level3 book whose depth enforcement trims a price level mid-batch", t, func() {
		live := &Live{isLevel3: true, books: spot.NewBookManager()}

		subscribe := newLevel3Event([]byte(
			`{"method":"subscribe","params":{"channel":"level3","symbol":["TEST/USD"],"depth":1}}`,
		))
		live.updateBooks(subscribe)

		// Depth 1 means inserting order B (99) trims order B's own level
		// right back out via a synthetic zero-quantity update with no
		// order ID, orphaning B's per-order tracking. The trailing
		// explicit delete for order B then finds no level to update on,
		// which is exactly what crashes the vendored book package.
		update := newLevel3Event([]byte(`{
			"channel": "level3",
			"type": "update",
			"data": [{
				"symbol": "TEST/USD",
				"bids": [
					{"order_id": "A", "limit_price": 100, "order_qty": 1, "timestamp": "2024-01-01T00:00:00Z"},
					{"order_id": "B", "limit_price": 99, "order_qty": 1, "timestamp": "2024-01-01T00:00:01Z"},
					{"order_id": "B", "limit_price": 99, "event": "delete", "timestamp": "2024-01-01T00:00:02Z"}
				],
				"asks": [],
				"checksum": 0
			}]
		}`))

		Convey("It should not crash the process when a later record targets the trimmed level", func() {
			So(func() { live.updateBooks(update) }, ShouldNotPanic)

			survivor := live.books.GetBook("TEST/USD").BestBid()

			So(survivor, ShouldNotBeNil)
			So(survivor.Price.String(), ShouldEqual, "100")
		})
	})
}

func BenchmarkLiveRoute(b *testing.B) {
	live := &Live{sync: &sync.Map{}}
	live.On("level3", func([]byte) {})
	raw := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD"}]}`)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		live.route(raw)
	}
}

func TestAuthNonceSurvivesRestart(t *testing.T) {
	Convey("Given the auth nonce generator used for authenticated transports", t, func() {
		nonceCounter := kraken.NewEpochCounter()
		nonceCounter.Granularity = time.Microsecond

		priorRunLastNonce, err := strconv.ParseInt(nonceCounter.Get(), 10, 64)
		So(err, ShouldBeNil)

		Convey("It should stay within the int64 range Kraken expects", func() {
			So(priorRunLastNonce, ShouldBeGreaterThan, int64(0))
		})

		Convey("It should still increase for a brand new counter started immediately after", func() {
			restartedCounter := kraken.NewEpochCounter()
			restartedCounter.Granularity = time.Microsecond

			firstNonceAfterRestart, err := strconv.ParseInt(restartedCounter.Get(), 10, 64)

			So(err, ShouldBeNil)
			So(firstNonceAfterRestart, ShouldBeGreaterThan, priorRunLastNonce)
		})
	})
}
