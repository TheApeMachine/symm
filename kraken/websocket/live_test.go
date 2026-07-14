package websocket

import (
	"context"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

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

func TestLiveUpdateLevel3PreservesMessageBoundary(t *testing.T) {
	Convey("Given a depth-limited L3 book and one atomic boundary shift", t, func() {
		live := New(context.Background(), nil, true, Level3WebSocketURL)
		live.client.Reconnect = func() {}
		managed := live.books.CreateBook("BTC/USD", 1)
		restingPrice, err := decimal.NewFromString("100")
		So(err, ShouldBeNil)

		managed.Update(&book.UpdateOptions{
			Direction: book.Bid,
			ID:        "resting",
			Price:     restingPrice,
			Quantity:  decimal.NewFromInt64(1),
			Timestamp: time.Unix(1, 0),
		})

		checksum := crc32.ChecksumIEEE([]byte("1011"))
		raw := []byte(fmt.Sprintf(`{
			"channel":"level3",
			"type":"update",
			"data":[{
				"symbol":"BTC/USD",
				"checksum":%d,
				"bids":[
					{"event":"add","order_id":"new-best","limit_price":101,"order_qty":1,"timestamp":"2024-01-01T00:00:02Z"},
					{"event":"delete","order_id":"resting","limit_price":100,"timestamp":"2024-01-01T00:00:03Z"}
				],
				"asks":[]
			}]
		}`, checksum))
		event := &callback.Event[*kraken.WebSocketMessage]{
			Data: kraken.NewWebSocketMessage(raw),
		}

		Convey("When the complete message is applied", func() {
			err := live.updateLevel3(event)

			Convey("Then no intermediate trim loses the later deleted level", func() {
				So(err, ShouldBeNil)
				So(managed.EnableMaxDepth, ShouldBeFalse)
				So(managed.NoBookCrossing, ShouldBeFalse)
				So(managed.Bids.Levels, ShouldHaveLength, 1)
				So(managed.BestBid().Price.Float64(), ShouldEqual, 101.0)
			})
		})
	})
}

func TestLiveSubscribeLevel3SendsDepth(t *testing.T) {
	Convey("Given an authenticated L3 websocket", t, func() {
		requests := make(chan map[string]any, 1)
		upgrader := gorillawebsocket.Upgrader{}
		server := httptest.NewServer(http.HandlerFunc(func(
			response http.ResponseWriter,
			request *http.Request,
		) {
			connection, err := upgrader.Upgrade(response, request, nil)

			if err != nil {
				return
			}

			defer connection.Close()

			var message map[string]any

			if connection.ReadJSON(&message) == nil {
				requests <- message
			}

			connection.ReadMessage()
		}))
		defer server.Close()

		client := spot.NewWebSocket()
		client.URL = "ws" + strings.TrimPrefix(server.URL, "http")
		client.Token = "token"
		live := &Live{client: client}

		So(client.Connect(), ShouldBeNil)

		Convey("When level3 is subscribed at the configured depth", func() {
			So(live.SubscribeLevel3([]string{"BTC/USD"}, 100), ShouldBeNil)

			request := <-requests
			params := request["params"].(map[string]any)

			Convey("Then depth is present on the actual wire request", func() {
				So(request["method"], ShouldEqual, "subscribe")
				So(params["channel"], ShouldEqual, "level3")
				So(params["depth"], ShouldEqual, float64(100))
				So(params["symbol"], ShouldResemble, []any{"BTC/USD"})
			})
		})

		So(client.Disconnect(), ShouldBeNil)
	})
}

/*
BenchmarkLiveUpdateLevel3 measures one complete L3 message application through
checksum validation and message-boundary depth enforcement.
*/
func BenchmarkLiveUpdateLevel3(b *testing.B) {
	live := New(context.Background(), nil, true, Level3WebSocketURL)
	live.client.Reconnect = func() {}
	live.books.CreateBook("BTC/USD", 10)
	checksum := crc32.ChecksumIEEE([]byte("1011"))
	raw := []byte(fmt.Sprintf(`{
		"channel":"level3",
		"type":"update",
		"data":[{
			"symbol":"BTC/USD",
			"checksum":%d,
			"bids":[
				{"event":"modify","order_id":"best","limit_price":101,"order_qty":1,"timestamp":"2024-01-01T00:00:02Z"}
			],
			"asks":[]
		}]
	}`, checksum))
	event := &callback.Event[*kraken.WebSocketMessage]{
		Data: kraken.NewWebSocketMessage(raw),
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := live.updateLevel3(event); err != nil {
			b.Fatal(err)
		}
	}
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

		Convey("It should route level3 market data to registered handlers", func() {
			raw := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD"}]}`)
			live.route(raw)

			So(level3Frames, ShouldResemble, [][]byte{raw})
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
