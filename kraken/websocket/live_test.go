package websocket

import (
	"context"
	"encoding/json"
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
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
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

func TestResubscribeL3(t *testing.T) {
	Convey("Given a level3 child reporting a checksum-diverged symbol", t, func() {
		requests, endpoint, closeServer := subscriptionConnection(t, 2)
		defer closeServer()

		client := spot.NewWebSocket()
		client.URL = endpoint
		client.Token = "fixture-token"
		So(client.Connect(), ShouldBeNil)

		pace := 25 * time.Millisecond
		viper.Set("market.l3_depth", 10)
		viper.Set("market.subscribe.pace", pace)
		Reset(func() {
			viper.Set("market.subscribe.pace", nil)
		})

		parent := &Live{level3: &sync.Map{}}
		child := &Live{client: client, book: NewBook(context.Background(), spot.NewNormalizer())}
		child.symbols = append(child.symbols, "DIVERGED/USD", "QUIET/USD")
		parent.level3.Store("group", child)

		Convey("The book should report divergence to the owning subscription manager", func() {
			child.book.resync("DIVERGED/USD")

			select {
			case <-time.After(time.Second):
				t.Fatal("divergence never reached the subscription owner")
			}
		})

		Convey("Recovery should pace between unsubscribe and fresh subscribe", func() {
			started := time.Now()
			elapsed := time.Since(started)
			first := <-requests
			second := <-requests
			firstMethod := first["method"].(string)
			secondParams := second["params"].(map[string]any)
			secondSymbols := secondParams["symbol"].([]any)

			So(firstMethod, ShouldEqual, "unsubscribe")
			So(secondParams["channel"].(string), ShouldEqual, "level3")
			So(secondSymbols, ShouldHaveLength, 1)
			So(secondSymbols[0], ShouldEqual, "DIVERGED/USD")
			So(elapsed, ShouldBeGreaterThanOrEqualTo, pace)
		})
	})
}

func TestRememberPublicSubscription(t *testing.T) {
	Convey("Given public subscriptions submitted in batches", t, func() {
		live := &Live{public: make(map[string][][]string)}

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

func TestLiveSubscribe(t *testing.T) {
	Convey("Given a paper-mode private transport", t, func() {
		paper := NewPaper(t.Context(), NewLatencySimulator(t.Context(), nil, 1))
		live := &Live{
			model:       "paper",
			paper:       paper,
			subscribers: &sync.Map{},
		}
		subscription := live.Subscribe("executions", &types.Subscription[any]{
			Channel: make(chan any, 1),
		})
		execution := &kraken.Execution{Channel: "executions", Type: "update"}

		Convey("A paper fill should reach the live execution subscriber", func() {
			paper.publish("executions", execution)
			So(<-subscription.Channel, ShouldEqual, execution)
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
