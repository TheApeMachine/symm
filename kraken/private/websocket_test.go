package private

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/types"
)

func TestWebSocketConnectReturnsCanceledContext(test *testing.T) {
	convey.Convey("Given a canceled private websocket context", test, func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		websocketClient := &WebSocket{
			ctx: ctx,
		}

		convey.Convey("It should return without dialing or sleeping", func() {
			connectErr := websocketClient.Connect(public.WebSocketAuthURL, 0)

			convey.So(errors.Is(connectErr, context.Canceled), convey.ShouldBeTrue)
			convey.So(websocketClient.isConnected.Load(), convey.ShouldBeFalse)
		})
	})
}

func TestWebSocketHandleErrorsMalformedService(test *testing.T) {
	convey.Convey("Given a malformed service error frame", test, func() {
		message := types.NewSocketMessage()
		message.Errors = []string{"EService"}

		websocketClient := &WebSocket{
			ctx: context.Background(),
		}

		convey.Convey("It should not panic while parsing the error", func() {
			convey.So(func() {
				websocketClient.handleErrors(message)
			}, convey.ShouldNotPanic)
		})
	})
}

func TestWebSocketReadLoopSkipsUnsupportedPaperMessages(test *testing.T) {
	convey.Convey("Given a paper private websocket read loop", test, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		defer pool.Close()

		websocketClient := &WebSocket{
			ctx:                  ctx,
			forwardPrivateOrders: false,
			conn:                 &websocket.Conn{},
			bus: internal.NewBus(
				ctx,
				pool,
				[]internal.Channel{internal.ChannelKrakenPrivate},
				[]internal.Subscription{
					internal.Subscribe(internal.ChannelKrakenPrivate, "private-readloop-test"),
				},
			),
		}
		websocketClient.isConnected.Store(true)

		done := make(chan struct{})

		go func() {
			defer close(done)
			websocketClient.readLoop()
		}()

		sendErr := websocketClient.bus.Send(
			internal.ChannelKrakenPrivate,
			"balances",
			types.KrakenMessage{},
		)

		convey.So(sendErr, convey.ShouldBeNil)

		cancel()

		select {
		case <-done:
		case <-time.After(time.Second):
			test.Fatal("private read loop did not exit after cancellation")
		}
	})
}

func TestNewWebSocketCredentialFailureDoesNotPoisonConstructor(test *testing.T) {
	convey.Convey("Given a failed private websocket construction", test, func() {
		testconfig.Load(test)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		defer pool.Close()

		test.Setenv("SYMM_KRAKEN_API_KEY", "")
		test.Setenv("SYMM_KRAKEN_API_SECRET", "")

		firstClient := NewWebSocket(ctx, pool)

		test.Setenv("SYMM_KRAKEN_API_KEY", "key")
		test.Setenv("SYMM_KRAKEN_API_SECRET", krakenDocPrivateKey)

		secondClient := NewWebSocket(ctx, pool)
		defer func() {
			if secondClient != nil {
				convey.So(secondClient.Close(), convey.ShouldBeNil)
			}
		}()

		convey.Convey("It should allow a later valid construction", func() {
			convey.So(firstClient, convey.ShouldBeNil)
			convey.So(secondClient, convey.ShouldNotBeNil)
		})
	})
}
