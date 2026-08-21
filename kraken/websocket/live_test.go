package websocket

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/book"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

type memoryCaptureSink struct {
	endpoint string
	payload  []byte
	at       time.Time
}

func (sink *memoryCaptureSink) Capture(
	endpoint string, payload []byte, receivedAt time.Time,
) error {
	sink.endpoint = endpoint
	sink.payload = payload
	sink.at = receivedAt

	return nil
}

func TestCaptureFrame(t *testing.T) {
	Convey("Given one raw public websocket payload", t, func() {
		sink := &memoryCaptureSink{}
		live := &Live{capture: sink, captureName: "public"}
		payload := []byte(`{"channel":"ticker","data":[{"symbol":"BTC/USD"}]}`)

		So(live.captureFrame("public", payload), ShouldBeNil)

		Convey("It should retain the untouched payload", func() {
			So(sink.endpoint, ShouldEqual, "public")
			So(string(sink.payload), ShouldEqual, string(payload))
			So(sink.at.IsZero(), ShouldBeFalse)
		})

		Convey("It should own its bytes when the SDK reuses the buffer", func() {
			captured := string(sink.payload)

			for index := range payload {
				payload[index] = 'X'
			}

			So(string(sink.payload), ShouldEqual, captured)
		})
	})
}

func TestTradeVolume(t *testing.T) {
	Convey("Given a live fee-tier response and market recorder", t, func() {
		response := `{"error":[],"result":{"fees":{"BTCUSD":{"fee":"0.26"}}}}`
		client := spot.NewWebSocket()
		client.REST.Executor = func(
			request *http.Request,
		) (*http.Response, error) {
			So(request.URL.Path, ShouldEqual, TradeVolumeEndpoint)

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(response)),
			}, nil
		}
		sink := &memoryCaptureSink{}
		live := &Live{client: client, capture: sink}

		result, err := live.TradeVolume([]string{"BTC/USD"})
		So(err, ShouldBeNil)
		So(result, ShouldNotBeNil)

		Convey("It should retain the exact REST response beside market frames", func() {
			So(sink.endpoint, ShouldEqual, TradeVolumeEndpoint)
			So(string(sink.payload), ShouldEqual, response)
		})
	})
}

func TestStatus(t *testing.T) {
	Convey("Given concurrent transport lifecycle changes", t, func() {
		live := &Live{}
		statuses := []types.Status{types.PENDING, types.READY, types.ERROR}
		var waitGroup sync.WaitGroup

		for _, status := range statuses {
			waitGroup.Add(1)

			go func(status types.Status) {
				defer waitGroup.Done()
				live.setStatus(status)
				_ = live.Status()
			}(status)
		}

		waitGroup.Wait()

		Convey("It should publish a complete atomic status", func() {
			So(statuses, ShouldContain, live.Status())
		})
	})
}

func TestSetObserver(t *testing.T) {
	Convey("Given a live session with an existing Level 3 child", t, func() {
		parent := &Live{level3: &sync.Map{}}
		child := &Live{}
		parent.level3.Store("BTC/USD", child)
		observed := make(chan string, 2)
		parent.SetObserver(func(name string, _ time.Duration) {
			observed <- name
		})

		parentObserver := parent.observer.Load()
		childObserver := child.observer.Load()

		So(parentObserver, ShouldNotBeNil)
		So(childObserver, ShouldNotBeNil)
		(*parentObserver)("crypto", time.Millisecond)
		(*childObserver)("crypto", time.Millisecond)

		Convey("Both ingress paths should report through the same clock", func() {
			So(<-observed, ShouldEqual, "crypto")
			So(<-observed, ShouldEqual, "crypto")
		})
	})
}

func subscriptionConnection(
	t *testing.T,
	requestCount int,
) (chan map[string]any, string, func()) {
	t.Helper()
	requests := make(chan map[string]any, requestCount)
	upgrader := gorillawebsocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(
		responseWriter http.ResponseWriter,
		request *http.Request,
	) {
		connection, err := upgrader.Upgrade(responseWriter, request, nil)

		if err != nil {
			return
		}

		defer connection.Close()

		for range requestCount {
			_, raw, err := connection.ReadMessage()

			if err != nil {
				return
			}

			wire := map[string]any{}

			if json.Unmarshal(raw, &wire) == nil {
				requests <- wire
			}
		}

		_ = connection.WriteMessage(
			gorillawebsocket.CloseMessage,
			gorillawebsocket.FormatCloseMessage(
				gorillawebsocket.CloseNormalClosure,
				"",
			),
		)
	}))
	return requests, "ws" + strings.TrimPrefix(server.URL, "http"), server.Close
}

func TestRestorePublicSubscriptions(t *testing.T) {
	Convey("Given remembered public websocket subscriptions", t, func() {
		requests, endpoint, closeServer := subscriptionConnection(t, 2)
		defer closeServer()

		client := spot.NewWebSocket()
		client.URL = endpoint
		So(client.Connect(), ShouldBeNil)
		live := &Live{
			client: client,
			public: map[string][][]string{
				"ticker": {{"BTC/USD"}},
				"trade":  {{"ETH/USD"}},
			},
		}
		So(live.restorePublicSubscriptions(), ShouldBeNil)

		Convey("A reconnect should restore every channel with its symbols", func() {
			channels := make([]string, 0, 2)

			for range 2 {
				request := <-requests
				params := request["params"].(map[string]any)
				channels = append(channels, params["channel"].(string))
			}

			slices.Sort(channels)
			So(channels, ShouldResemble, []string{"ticker", "trade"})
		})
	})
}

func TestRememberPublicSubscription(t *testing.T) {
	Convey("Given public subscriptions submitted in batches", t, func() {
		live := &Live{public: make(map[string][][]string)}
		live.rememberPublicSubscription("ticker", []string{"BTC/USD", "ETH/USD"})
		live.rememberPublicSubscription("ticker", []string{"ADA/USD"})

		Convey("Distinct symbols and original request boundaries should remain available for reconnect", func() {
			So(live.public["ticker"], ShouldResemble, [][]string{
				{"BTC/USD", "ETH/USD"},
				{"ADA/USD"},
			})
		})
	})
}

func TestLiveBook(t *testing.T) {
	Convey("Given a Level 3 connection containing the requested book", t, func() {
		managed := newBookFixture(t, "BTC/USD", 0, 0)
		managed.Create("BTC/USD", 32)
		live := &Live{level3: &sync.Map{}}
		live.level3.Store("unrelated subscription key", &Live{book: managed})

		Convey("It should find the managed book without parsing subscription keys", func() {
			live.Book("BTC/USD", func(actual *book.Book) {
				managed.Get("BTC/USD", func(expected *book.Book) {
					So(actual, ShouldEqual, expected)
				})
			})
		})
	})
}

func TestNewWithClient(t *testing.T) {
	Convey("Given a Level3 child with one resident book", t, func() {
		assets := `{"error":[],"result":{"BTC":{"altname":"BTC"},"USD":{"altname":"USD"}}}`
		pairs := `{"error":[],"result":{"BTCUSD":{"altname":"BTCUSD","wsname":"BTC/USD","base":"BTC","quote":"USD","pair_decimals":1,"lot_decimals":8,"lot_multiplier":1,"tick_size":"0.1"}}}`
		client := spot.NewWebSocket()
		client.REST.Executor = func(request *http.Request) (*http.Response, error) {
			body := assets

			if request.URL.Path == "/0/public/AssetPairs" {
				body = pairs
			}

			if request.URL.Path != "/0/public/Assets" &&
				request.URL.Path != "/0/public/AssetPairs" {
				return nil, fmt.Errorf("unexpected normalizer request: %s", request.URL.Path)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}

		upgrader := gorillawebsocket.Upgrader{}
		server := httptest.NewServer(http.HandlerFunc(func(
			responseWriter http.ResponseWriter,
			request *http.Request,
		) {
			connection, err := upgrader.Upgrade(responseWriter, request, nil)

			if err != nil {
				return
			}

			defer connection.Close()

			for {
				if _, _, err := connection.ReadMessage(); err != nil {
					return
				}
			}
		}))
		defer server.Close()
		client.URL = "ws" + strings.TrimPrefix(server.URL, "http")

		thesis := types.NewThesis(t.Context(), nil)
		defer thesis.Close()
		child := NewWithClient(
			t.Context(), thesis, nil, false, Level3WebSocketURL, client,
		)
		So(child, ShouldNotBeNil)
		defer child.Close()
		child.book.Create("BTC/USD", 10)

		resident := thesis.Symbol("BTC/USD")
		nonresident := thesis.Symbol("ETH/USD")
		residentDepthFlow := resident.Level3Consumers[types.Level3ConsumerDepthFlow]
		nonresidentDepthFlow := nonresident.Level3Consumers[types.Level3ConsumerDepthFlow]
		notifications := 0
		residentQueuedAtNotification := false
		nonresidentQueuedAtNotification := false
		touchReadyAtNotification := false
		workConsumer := transport.NewConsumer[*types.Symbol]("level3-routing-test", func() {
			notifications++
			residentQueuedAtNotification = resident.HasLevel3For(residentDepthFlow)
			nonresidentQueuedAtNotification = nonresident.HasLevel3For(nonresidentDepthFlow)
			child.book.Get("BTC/USD", func(current *book.Book) {
				touchReadyAtNotification = current.BestBid() != nil && current.BestAsk() != nil
			})
		})
		thesis.Work(types.SourceDepthFlow).Register(workConsumer)

		raw := []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"ETH/USD","timestamp":"2024-01-01T00:00:00Z","bids":[{"order_id":"foreign-bid","limit_price":"10.0","order_qty":"1.00000000","timestamp":"2024-01-01T00:00:00Z"}],"asks":[{"order_id":"foreign-ask","limit_price":"11.0","order_qty":"1.00000000","timestamp":"2024-01-01T00:00:00Z"}]},{"symbol":"BTC/USD","timestamp":"2024-01-01T00:00:01Z","bids":[{"order_id":"resident-bid","limit_price":"100.0","order_qty":"2.00000000","timestamp":"2024-01-01T00:00:01Z"}],"asks":[{"order_id":"resident-ask","limit_price":"101.0","order_qty":"3.00000000","timestamp":"2024-01-01T00:00:01Z"}]}]}`)
		client.OnReceived.Call(sdkkraken.NewWebSocketMessage(raw))

		Convey("Only the accepted resident frame should wake readers after its touch is executable", func() {
			So(notifications, ShouldEqual, 1)
			So(residentQueuedAtNotification, ShouldBeTrue)
			So(nonresidentQueuedAtNotification, ShouldBeFalse)
			So(touchReadyAtNotification, ShouldBeTrue)

			accepted := make([]kraken.Level3Data, 0, 1)

			for frame := range resident.MarketLevel3(residentDepthFlow) {
				accepted = append(accepted, frame)
			}

			So(accepted, ShouldHaveLength, 1)
			So(accepted[0].Symbol, ShouldEqual, "BTC/USD")
			So(accepted[0].Bids, ShouldHaveLength, 1)
			So(accepted[0].Asks, ShouldHaveLength, 1)
			So(nonresident.HasLevel3For(nonresidentDepthFlow), ShouldBeFalse)
		})
	})
}

func TestSubscribeAccount(t *testing.T) {
	Convey("Given an authenticated private websocket", t, func() {
		requests := make(chan map[string]any, 2)
		serverErrors := make(chan error, 1)
		upgrader := gorillawebsocket.Upgrader{}
		server := httptest.NewServer(http.HandlerFunc(func(
			responseWriter http.ResponseWriter,
			request *http.Request,
		) {
			connection, err := upgrader.Upgrade(responseWriter, request, nil)

			if err != nil {
				serverErrors <- err
				return
			}

			defer connection.Close()

			for range 2 {
				_, raw, err := connection.ReadMessage()

				if err != nil {
					serverErrors <- err
					return
				}

				wire := map[string]any{}

				if err := json.Unmarshal(raw, &wire); err != nil {
					serverErrors <- err
					return
				}

				requests <- wire
			}

			_ = connection.WriteMessage(
				gorillawebsocket.CloseMessage,
				gorillawebsocket.FormatCloseMessage(
					gorillawebsocket.CloseNormalClosure,
					"",
				),
			)
		}))
		defer server.Close()

		client := spot.NewWebSocket()
		client.URL = "ws" + strings.TrimPrefix(server.URL, "http")
		So(client.Connect(), ShouldBeNil)

		live := &Live{client: client}

		Convey("When the account streams are subscribed", func() {
			So(live.subscribeAccount("private-token"), ShouldBeNil)

			balanceRequest := <-requests
			executionRequest := <-requests
			balanceParams := balanceRequest["params"].(map[string]any)
			executionParams := executionRequest["params"].(map[string]any)

			So(balanceRequest["method"], ShouldEqual, "subscribe")
			So(balanceParams["channel"], ShouldEqual, "balances")
			So(balanceParams["token"], ShouldEqual, "private-token")
			So(executionRequest["method"], ShouldEqual, "subscribe")
			So(executionParams["channel"], ShouldEqual, "executions")
			So(executionParams["token"], ShouldEqual, "private-token")

			select {
			case err := <-serverErrors:
				So(err, ShouldBeNil)
			default:
			}

			So(entityMap["balances"]([]byte(`{"channel":"balances"}`)), ShouldHaveSameTypeAs, &kraken.Balance{})
			So(entityMap["executions"]([]byte(`{"channel":"executions"}`)), ShouldHaveSameTypeAs, &kraken.Execution{})
			So(entityMap["ohlc"]([]byte(`{"channel":"ohlc"}`)), ShouldHaveSameTypeAs, &kraken.OHLC{})
		})
	})
}

func BenchmarkLiveBook(b *testing.B) {
	managed := newBookFixture(b, "BTC/USD", 0, 0)
	managed.Create("BTC/USD", 32)
	live := &Live{level3: &sync.Map{}}
	live.level3.Store("subscription", &Live{book: managed})
	read := func(*book.Book) {}
	b.ReportAllocs()

	for b.Loop() {
		live.Book("BTC/USD", read)
	}
}

func BenchmarkCaptureFrame(b *testing.B) {
	sink := &memoryCaptureSink{}
	live := &Live{capture: sink, captureName: "public"}
	payload := []byte(`{"channel":"ticker","data":[{"symbol":"BTC/USD"}]}`)
	b.ReportAllocs()

	for b.Loop() {
		if err := live.captureFrame("public", payload); err != nil {
			b.Fatal(err)
		}
	}
}
