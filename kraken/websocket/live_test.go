package websocket

import (
	"context"
	"fmt"
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

func TestLiveRoutesPrivateChannels(t *testing.T) {
	Convey("Given an authenticated private websocket transport", t, func() {
		live := New(context.Background(), nil, true, PrivateWebSocketURL)
		defer live.Close()
		balances := live.Subscribe("balances")
		executions := live.Subscribe("executions")
		orders := live.Subscribe("add_order")
		balanceRaw := []byte(`{"channel":"balances","type":"snapshot","data":[]}`)
		executionRaw := []byte(`{"channel":"executions","type":"update","data":[]}`)
		orderRaw := []byte(`{"method":"add_order","success":true,"result":{"order_id":"OID-1"},"req_id":7}`)

		live.client.OnReceived.Call(kraken.NewWebSocketMessage(balanceRaw))
		live.client.OnReceived.Call(kraken.NewWebSocketMessage(executionRaw))
		live.client.OnReceived.Call(kraken.NewWebSocketMessage(orderRaw))

		Convey("Then private channel payloads reach their actor roots unchanged", func() {
			So((<-balances.Channel).([]byte), ShouldResemble, balanceRaw)
			So((<-executions.Channel).([]byte), ShouldResemble, executionRaw)
			So((<-orders.Channel).([]byte), ShouldResemble, orderRaw)
		})
	})
}

func TestLiveApplyLevel3(t *testing.T) {
	Convey("Given a standard SDK-managed level3 book", t, func() {
		live := New(context.Background(), nil, true, Level3WebSocketURL)
		live.client.Reconnect = func() {}

		So(live.ApplyLevel3([]byte(
			`{"method":"subscribe","params":{"channel":"level3","symbol":["BTC/USD"],"depth":10}}`,
		)), ShouldBeNil)

		scratch := spot.NewBookManager()
		managed := scratch.CreateBook("BTC/USD", 10)
		managed.EnableMaxDepth = false
		managed.NoBookCrossing = false
		managed.Update(&book.UpdateOptions{
			Direction: book.Bid,
			ID:        "bid-1",
			Price:     decimal.NewFromFloat64(100),
			Quantity:  decimal.NewFromFloat64(1),
			Timestamp: time.Unix(1, 0),
		})
		checksum := managed.L3Checksum("").LocalChecksum
		snapshot := fmt.Appendf(nil, `{
			"channel":"level3",
			"type":"snapshot",
			"data":[{
				"symbol":"BTC/USD",
				"checksum":%s,
				"bids":[{"order_id":"bid-1","limit_price":100,"order_qty":1,"timestamp":"1970-01-01T00:00:01Z"}],
				"asks":[]
			}]
		}`, checksum)

		payload := fmt.Appendf(nil, `{
			"channel":"level3",
			"type":"update",
			"data":[{
				"symbol":"BTC/USD",
				"checksum":%s,
				"bids":[{"event":"modify","order_id":"bid-1","limit_price":100,"order_qty":1,"timestamp":"1970-01-01T00:00:02Z"}],
				"asks":[]
			}]
		}`, checksum)

		Convey("When one level3 frame is applied", func() {
			So(live.ApplyLevel3(snapshot), ShouldBeNil)
			So(live.ApplyLevel3(payload), ShouldBeNil)

			Convey("Then the SDK BookManager exposes the updated resting book", func() {
				symbolBook := live.books.GetBook("BTC/USD")
				So(symbolBook, ShouldNotBeNil)
				So(symbolBook.BestBid(), ShouldNotBeNil)
				So(symbolBook.BestBid().Price.Float64(), ShouldEqual, 100.0)
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
BenchmarkLiveApplyLevel3 measures one complete L3 message application through
the standard SDK BookManager path.
*/
func BenchmarkLiveApplyLevel3(b *testing.B) {
	live := New(context.Background(), nil, true, Level3WebSocketURL)
	live.client.Reconnect = func() {}
	if err := live.ApplyLevel3([]byte(
		`{"method":"subscribe","params":{"channel":"level3","symbol":["BTC/USD"],"depth":10}}`,
	)); err != nil {
		b.Fatal(err)
	}

	scratch := spot.NewBookManager()
	managed := scratch.CreateBook("BTC/USD", 10)
	managed.EnableMaxDepth = false
	managed.NoBookCrossing = false
	managed.Update(&book.UpdateOptions{
		Direction: book.Bid,
		ID:        "best",
		Price:     decimal.NewFromFloat64(101),
		Quantity:  decimal.NewFromFloat64(1),
		Timestamp: time.Unix(1, 0),
	})
	checksum := managed.L3Checksum("").LocalChecksum
	raw := fmt.Appendf(nil, `{
		"channel":"level3",
		"type":"update",
		"data":[{
			"symbol":"BTC/USD",
			"checksum":%s,
			"bids":[
				{"event":"modify","order_id":"best","limit_price":101,"order_qty":1,"timestamp":"2024-01-01T00:00:02Z"}
			],
			"asks":[]
		}]
	}`, checksum)

	b.ReportAllocs()

	for b.Loop() {
		if err := live.ApplyLevel3(raw); err != nil {
			b.Fatal(err)
		}
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
