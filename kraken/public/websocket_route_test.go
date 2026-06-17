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
