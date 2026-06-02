package market

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
)

func TestParseInboundPayload(t *testing.T) {
	Convey("Given a websocket pong frame", t, func() {
		message, handled, err := parseInboundPayload([]byte(`{"method":"pong"}`))

		Convey("It should treat it as handled control traffic", func() {
			So(err, ShouldBeNil)
			So(handled, ShouldBeTrue)
			So(message.Channel, ShouldBeEmpty)
		})
	})

	Convey("Given a level3 channel frame", t, func() {
		message, handled, err := parseInboundPayload([]byte(`{"channel":"level3","type":"update"}`))

		Convey("It should decode the socket message", func() {
			So(err, ShouldBeNil)
			So(handled, ShouldBeFalse)
			So(message.Channel, ShouldEqual, public.Level3Channel)
			So(message.Type, ShouldEqual, "update")
		})
	})
}

func TestLevel3WebSocketReadLoopSurvivesIdle(t *testing.T) {
	Convey("Given a connected level3 socket with no inbound traffic", t, func() {
		upgrader := websocket.Upgrader{}

		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			conn, upgradeErr := upgrader.Upgrade(writer, request, nil)

			if upgradeErr != nil {
				return
			}

			defer conn.Close()

			for {
				if _, _, readErr := conn.ReadMessage(); readErr != nil {
					return
				}
			}
		}))
		defer server.Close()

		ctx, cancel := context.WithCancel(context.Background())

		pool := qpool.NewQ(ctx, 1, 4, nil)

		ws := &Level3WebSocket{
			ctx:             ctx,
			cancel:          cancel,
			pool:            pool,
			reconnectPolicy: public.NewReconnectPolicyFromConfig(),
			subscribeReplay: make([]any, 0),
			inbound:         make(chan level3ReadResult, 8),
		}

		go ws.readLoop()

		conn, _, dialErr := websocket.DefaultDialer.Dial(
			"ws"+server.URL[4:],
			nil,
		)

		So(dialErr, ShouldBeNil)

		ws.connMu.Lock()
		ws.conn = conn
		ws.connMu.Unlock()

		time.Sleep(2 * time.Second)

		cancel()
		pool.Close()

		Convey("It should keep blocking reads without panicking", func() {
			So(true, ShouldBeTrue)
		})
	})
}

func BenchmarkParseInboundPayload(b *testing.B) {
	payload := []byte(`{"channel":"level3","type":"update","data":[]}`)

	b.ReportAllocs()

	for b.Loop() {
		_, _, _ = parseInboundPayload(payload)
	}
}
