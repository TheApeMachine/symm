package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

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
