package public

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
)

func testPool(testingTB testing.TB) *qpool.Q[any] {
	testingTB.Helper()

	return qpool.NewQ[any](context.Background(), 2, 4, &qpool.Config{Scaler: nil})
}

func TestWebSocketOnMessageRoutesOutbound(testingTB *testing.T) {
	Convey("Given a public websocket client", testingTB, func() {
		socket := NewWebSocket(context.Background(), testPool(testingTB))
		payload, marshalErr := sonic.Marshal(map[string]any{
			"method": "subscribe",
			"params": map[string]any{"channel": "ticker"},
		})

		So(marshalErr, ShouldBeNil)

		artifact := datura.Acquire("test", datura.Artifact_Type_json).
			WithDestination("kraken:public").
			WithPayload(payload)

		Convey("It should accept kraken:public destinations without error", func() {
			routeErr := socket.onMessage(artifact)

			So(routeErr, ShouldBeNil)
		})
	})
}

func TestWebSocketOnMessageRejectsUnknownDestination(testingTB *testing.T) {
	Convey("Given an artifact with an unknown destination", testingTB, func() {
		socket := NewWebSocket(context.Background(), testPool(testingTB))
		artifact := datura.Acquire("test", datura.Artifact_Type_json).
			WithDestination("unknown:channel").
			WithPayload([]byte(`{}`))

		Convey("It should reject the route", func() {
			routeErr := socket.onMessage(artifact)

			So(routeErr, ShouldNotBeNil)
		})
	})
}

func TestSocketMessageDecodeTickerRoute(testingTB *testing.T) {
	Convey("Given a Kraken ticker frame", testingTB, func() {
		message := types.Acquire()

		defer message.Release()

		observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		tickerPayload, marshalErr := sonic.Marshal(krakenmarket.TickerUpdates{{
			Symbol:    "BTC/USD",
			Last:      50000,
			Timestamp: observedAt,
		}})

		So(marshalErr, ShouldBeNil)

		frame := []byte(`{"channel":"ticker","type":"update","success":true,"data":` +
			string(tickerPayload) + `}`)

		decodeErr := message.Decode(frame)

		Convey("It should decode channel metadata and payload", func() {
			So(decodeErr, ShouldBeNil)
			So(message.Channel, ShouldEqual, "ticker")
			So(message.Type, ShouldEqual, "update")
			So(message.Success, ShouldBeTrue)

			updates := krakenmarket.TickerUpdates{}
			unmarshalErr := message.Unmarshal(&updates)

			So(unmarshalErr, ShouldBeNil)
			So(len(updates), ShouldEqual, 1)
			So(updates[0].Symbol, ShouldEqual, "BTC/USD")
		})
	})
}

func TestWebSocketRouteInboundSkipsEmptyPayload(testingTB *testing.T) {
	Convey("Given a subscribe acknowledgement without data", testingTB, func() {
		socket := NewWebSocket(context.Background(), testPool(testingTB))
		message := types.Acquire()

		defer message.Release()

		decodeErr := message.Decode([]byte(
			`{"method":"subscribe","success":true,"time_in":"2026-06-17T08:25:22Z","time_out":"2026-06-17T08:25:22Z"}`,
		))

		So(decodeErr, ShouldBeNil)

		Convey("It should skip routing without panicking", func() {
			So(func() { socket.routeInbound(message) }, ShouldNotPanic)
		})
	})
}

func TestWebSocketRouteInboundTicker(testingTB *testing.T) {
	Convey("Given a ticker frame with payload", testingTB, func() {
		ctx := context.Background()
		pool := testPool(testingTB)
		socket := NewWebSocket(ctx, pool)

		received := make(chan string, 1)

		pool.Subscribe("ticker", func(artifact *datura.Artifact) error {
			received <- datura.Peek[string](artifact, "scope")

			return nil
		})

		message := types.Acquire()

		defer message.Release()

		tickerPayload, marshalErr := sonic.Marshal(krakenmarket.TickerUpdates{{
			Symbol: "BTC/USD",
			Last:   50000,
		}})

		So(marshalErr, ShouldBeNil)

		frame := []byte(`{"channel":"ticker","type":"update","success":true,"data":` +
			string(tickerPayload) + `}`)

		So(message.Decode(frame), ShouldBeNil)

		Convey("When routeInbound is called", func() {
			socket.routeInbound(message)

			Convey("It should deliver the frame scope to subscribers", func() {
				var scope string

				select {
				case scope = <-received:
				case <-time.After(2 * time.Second):
					So("ticker frame", ShouldEqual, "received")
				}

				So(scope, ShouldEqual, "update")
			})

			Convey("It should index ticker rows in the market tree", func() {
				var found bool

				for inbound := range krakenmarket.MarketTree().Seek([]byte("ticker/BTC/USD")) {
					found = true
					inbound.Release()
				}

				So(found, ShouldBeTrue)
			})
		})
	})
}
