package websocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	gorillawebsocket "github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func subscriptionConnection(
	t *testing.T,
	requestCount int,
) (chan map[string]any, *gorillawebsocket.Conn, func()) {
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
	}))
	connection, _, err := gorillawebsocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http"), nil,
	)
	So(err, ShouldBeNil)

	return requests, connection, server.Close
}

func TestRestorePublicSubscriptions(t *testing.T) {
	Convey("Given remembered public websocket subscriptions", t, func() {
		requests, connection, closeServer := subscriptionConnection(t, 2)
		defer closeServer()
		defer connection.Close()

		client := spot.NewWebSocket()
		client.Conn = connection
		live := &Live{
			client: client,
			public: map[string][][]string{
				"ticker": {{"BTC/USD"}},
				"trade":  {{"ETH/USD"}},
			},
		}

		Convey("A reconnect should restore every channel with its symbols", func() {
			live.restorePublicSubscriptions()
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

		Convey("Distinct symbols and original request boundaries should remain available for reconnect", func() {
			live.rememberPublicSubscription("ticker", []string{"BTC/USD", "ETH/USD"})
			live.rememberPublicSubscription("ticker", []string{"ETH/USD", "ADA/USD"})
			So(live.public["ticker"], ShouldResemble, [][]string{
				{"BTC/USD", "ETH/USD"},
				{"ADA/USD"},
			})
		})
	})
}

func TestRestoreLevel3Subscription(t *testing.T) {
	Convey("Given remembered Level 3 symbols", t, func() {
		requests, connection, closeServer := subscriptionConnection(t, 2)
		defer closeServer()
		defer connection.Close()

		client := spot.NewWebSocket()
		client.Conn = connection
		symbols := make([]string, 41)

		for index := range symbols {
			symbols[index] = "SIM" + fmt.Sprint(index) + "/USD"
		}

		live := &Live{client: client, symbols: symbols}

		Convey("A reconnect should restore the symbols in Kraken-sized chunks", func() {
			live.restoreLevel3Subscription()
			first := <-requests
			second := <-requests
			firstSymbols := first["params"].(map[string]any)["symbol"].([]any)
			secondSymbols := second["params"].(map[string]any)["symbol"].([]any)
			So(len(firstSymbols), ShouldEqual, 40)
			So(len(secondSymbols), ShouldEqual, 1)
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
		}))
		defer server.Close()

		client := spot.NewWebSocket()
		connection, _, err := gorillawebsocket.DefaultDialer.Dial(
			"ws"+strings.TrimPrefix(server.URL, "http"), nil,
		)
		So(err, ShouldBeNil)
		defer connection.Close()
		client.Conn = connection

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
		subscription := live.Subscribe("executions", types.NewSubscription[any]())
		execution := &kraken.Execution{Channel: "executions", Type: "update"}

		Convey("A paper fill should reach the live execution subscriber", func() {
			paper.publish("executions", execution)
			So(<-subscription.Channel, ShouldEqual, execution)
		})
	})
}
