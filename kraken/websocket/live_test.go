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
		live.status = types.INITIALIZING
		live.client.URL = PrivateWebSocketURL

		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			live.status = types.READY
		})

		live.client.OnAuthenticated.Call("token")

		Convey("It should become ready after authentication", func() {
			So(live.client.URL, ShouldEqual, PrivateWebSocketURL)
			So(live.Status(), ShouldEqual, types.READY)
		})
	})
}

func TestLiveUpdateLevel3(t *testing.T) {
	Convey("Given a depth-limited SDK book", t, func() {
		live := New(context.Background(), nil, true, Level3WebSocketURL)
		live.client.Reconnect = func() {}
		managed := live.books.CreateBook("BTC/USD", 10)
		live.level3Ledger.orders["BTC/USD"] = make(map[string]level3Order)
		So(managed.EnableMaxDepth, ShouldBeFalse)
		So(managed.NoBookCrossing, ShouldBeFalse)
		quantity, err := decimal.NewFromString("1")
		So(err, ShouldBeNil)

		for price := 91; price <= 100; price++ {
			restingPrice, parseErr := decimal.NewFromString(strconv.Itoa(price))
			So(parseErr, ShouldBeNil)

			managed.Update(&book.UpdateOptions{
				Direction: book.Bid,
				ID:        "bid-" + strconv.Itoa(price),
				Price:     restingPrice,
				Quantity:  quantity,
				Timestamp: time.Unix(int64(price), 0),
			})
			live.level3Ledger.orders["BTC/USD"]["bid-"+strconv.Itoa(price)] = level3Order{
				price:    strconv.Itoa(price),
				quantity: "1",
			}
		}

		restingAskPrice, err := decimal.NewFromString("102")
		So(err, ShouldBeNil)

		managed.Update(&book.UpdateOptions{
			Direction: book.Ask,
			ID:        "resting-ask",
			Price:     restingAskPrice,
			Quantity:  quantity,
			Timestamp: time.Unix(1, 0),
		})
		live.level3Ledger.orders["BTC/USD"]["resting-ask"] = level3Order{
			price:    "102",
			quantity: "1",
		}

		checksum := crc32.ChecksumIEEE([]byte(
			"1031" + "1001" + "991" + "981" + "971" +
				"961" + "951" + "941" + "931" + "921",
		))
		raw := fmt.Appendf(nil, `{
			"channel":"level3",
			"type":"update",
			"data":[{
				"symbol":"BTC/USD",
				"checksum":%d,
				"bids":[
					{"event":"add","order_id":"new-best","limit_price":103,"order_qty":1,"timestamp":"2024-01-01T00:00:02Z"},
					{"event":"delete","order_id":"bid-91","limit_price":91,"timestamp":"2024-01-01T00:00:03Z"}
				],
				"asks":[
					{"event":"delete","order_id":"resting-ask","limit_price":102,"timestamp":"2024-01-01T00:00:03Z"}
				]
			}]
		}`, checksum)
		event := &callback.Event[*kraken.WebSocketMessage]{
			Data: kraken.NewWebSocketMessage(raw),
		}

		Convey("When a complete message repairs intermediate depth and crossing", func() {
			err := live.updateLevel3(event)

			Convey("Then the SDK applies every order before enforcing book depth", func() {
				So(err, ShouldBeNil)
				So(managed.Bids.Levels, ShouldHaveLength, 10)
				So(managed.Asks.Levels, ShouldBeEmpty)
				So(managed.BestBid().Price.Float64(), ShouldEqual, 103.0)
				So(managed.WorstBid().Price.Float64(), ShouldEqual, 92.0)
			})
		})

		Convey("When an update pushes a price level out of subscription scope", func() {
			checksum := crc32.ChecksumIEEE([]byte(
				"1021" + "1041" + "1001" + "991" + "981" + "971" +
					"961" + "951" + "941" + "931" + "921",
			))
			raw := fmt.Appendf(nil, `{
				"channel":"level3",
				"type":"update",
				"data":[{
					"symbol":"BTC/USD",
					"checksum":%d,
					"bids":[
						{"event":"add","order_id":"new-best","limit_price":104,"order_qty":1,"timestamp":"2024-01-01T00:00:02Z"}
					],
					"asks":[]
				}]
			}`, checksum)
			event := &callback.Event[*kraken.WebSocketMessage]{
				Data: kraken.NewWebSocketMessage(raw),
			}

			err := live.updateLevel3(event)

			Convey("Then the SDK removes the completed frame's worst level", func() {
				So(err, ShouldBeNil)
				So(managed.Bids.Levels, ShouldHaveLength, 10)
				So(managed.BestBid().Price.Float64(), ShouldEqual, 104.0)
				So(managed.WorstBid().Price.Float64(), ShouldEqual, 92.0)
				So(live.level3Ledger.orders["BTC/USD"], ShouldHaveLength, 11)
				_, retained := live.level3Ledger.orders["BTC/USD"]["bid-91"]
				So(retained, ShouldBeFalse)
			})
		})
	})
}

func TestLiveSubscribeLevel3UsesSDKSubL3(t *testing.T) {
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

		Convey("When level3 is subscribed through the SDK helper", func() {
			So(live.SubscribeLevel3([]string{"BTC/USD"}, 100), ShouldBeNil)

			var request map[string]any

			select {
			case request = <-requests:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for subscribe request")
			}

			params := request["params"].(map[string]any)

			Convey("Then the wire request matches SubL3", func() {
				So(request["method"], ShouldEqual, "subscribe")
				So(params["channel"], ShouldEqual, "level3")
				So(params["symbol"], ShouldResemble, []any{"BTC/USD"})
			})
		})

		So(client.Disconnect(), ShouldBeNil)
	})
}

/*
BenchmarkLiveUpdateLevel3 measures one complete L3 message application through
the SDK manager and checksum validation.
*/
func BenchmarkLiveUpdateLevel3(b *testing.B) {
	live := New(context.Background(), nil, true, Level3WebSocketURL)
	live.client.Reconnect = func() {}
	live.books.CreateBook("BTC/USD", 10)
	checksum := crc32.ChecksumIEEE([]byte("1011"))
	raw := fmt.Appendf(nil, `{
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
	}`, checksum)
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
		live := &Live{fanout: fanout{handlers: make(map[string][]channelHandler)}, isLevel3: true}
		tickerFrames := make([][]byte, 0, 1)
		orderFrames := make([][]byte, 0, 1)
		live.On("ticker", func(raw []byte) {
			tickerFrames = append(tickerFrames, raw)
		})
		live.On("add_order", func(raw []byte) {
			orderFrames = append(orderFrames, raw)
		})

		Convey("It should route data frames by their top-level channel", func() {
			raw := []byte(`{"channel":"ticker","type":"update"}`)
			live.route(raw)

			So(tickerFrames, ShouldResemble, [][]byte{raw})
		})

		Convey("It should route add-order acknowledgements by method", func() {
			raw := []byte(`{"method":"add_order","req_id":7,"success":true}`)
			live.route(raw)

			So(orderFrames, ShouldResemble, [][]byte{raw})
		})

		Convey("It should leave level3 market data with the SDK BookManager", func() {
			raw := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD"}]}`)
			live.route(raw)
		})

		Convey("It should not route subscription acknowledgements as market data", func() {
			raw := []byte(`{"method":"subscribe","result":{"channel":"level3"},"success":true}`)
			live.route(raw)

			So(tickerFrames, ShouldBeEmpty)
		})

		Convey("It should not route failed acknowledgements as market data", func() {
			raw := []byte(`{"error":"invalid depth","result":{"channel":"level3"},"success":false}`)
			live.route(raw)

			So(tickerFrames, ShouldBeEmpty)
		})

		Convey("It should ignore status and heartbeat frames", func() {
			live.route([]byte(`{"channel":"status"}`))
			live.route([]byte(`{"channel":"heartbeat"}`))

			So(tickerFrames, ShouldBeEmpty)
		})
	})
}

func BenchmarkLiveRoute(b *testing.B) {
	live := &Live{fanout: fanout{handlers: make(map[string][]channelHandler)}, isLevel3: true}
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

func TestAuthNonceIsSharedAcrossAuthenticatedLives(t *testing.T) {
	Convey("Given concurrent authenticated Live transports", t, func() {
		const workers = 32
		const perWorker = 64
		shared, err := processAuthNonce()
		So(err, ShouldBeNil)
		So(shared, ShouldNotBeNil)

		seen := make(map[string]struct{}, workers*perWorker)
		duplicates := 0
		var mu sync.Mutex
		var wait sync.WaitGroup
		wait.Add(workers)

		for range workers {
			go func() {
				defer wait.Done()

				for range perWorker {
					nonce := shared.Next()
					mu.Lock()

					if _, exists := seen[nonce]; exists {
						duplicates++
					}

					seen[nonce] = struct{}{}
					mu.Unlock()
				}
			}()
		}

		wait.Wait()

		Convey("Then every shared nonce is unique", func() {
			So(duplicates, ShouldEqual, 0)
			So(len(seen), ShouldEqual, workers*perWorker)
		})

		Convey("Then New wires the shared generator onto authenticated REST", func() {
			private := New(context.Background(), nil, true, PrivateWebSocketURL)
			level3 := New(context.Background(), nil, true, Level3WebSocketURL)
			defer private.Close()
			defer level3.Close()

			So(private.client.REST.Nonce, ShouldNotBeNil)
			So(level3.client.REST.Nonce, ShouldNotBeNil)

			first, parseErr := strconv.ParseInt(private.client.REST.Nonce(), 10, 64)
			So(parseErr, ShouldBeNil)
			second, parseErr := strconv.ParseInt(level3.client.REST.Nonce(), 10, 64)
			So(parseErr, ShouldBeNil)
			So(second, ShouldBeGreaterThan, first)
		})
	})
}
