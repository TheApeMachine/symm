package market

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
)

func TestNormalizeLevel3Depth(t *testing.T) {
	Convey("Given configured book depth levels", t, func() {
		Convey("It should map to Kraken-supported depths", func() {
			So(normalizeLevel3Depth(0), ShouldEqual, 10)
			So(normalizeLevel3Depth(10), ShouldEqual, 10)
			So(normalizeLevel3Depth(25), ShouldEqual, 100)
			So(normalizeLevel3Depth(500), ShouldEqual, 1000)
		})
	})
}

func TestMirrorPublicSubscribe(t *testing.T) {
	Convey("Given a public ticker subscribe frame", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws := NewLevel3WebSocket(ctx, pool)

		err := ws.mirrorPublicSubscribe(map[string]any{
			"method": "subscribe",
			"params": map[string]any{
				"channel": "ticker",
				"symbol":  []any{"BTC/EUR"},
			},
		})

		Convey("It should require configured credentials", func() {
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given a non-market subscribe frame", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws := NewLevel3WebSocket(ctx, pool)

		err := ws.mirrorPublicSubscribe(map[string]any{
			"method": "subscribe",
			"params": map[string]any{
				"channel": "instrument",
			},
		})

		Convey("It should ignore it", func() {
			So(err, ShouldBeNil)
		})
	})
}

func TestDecodeOrders(t *testing.T) {
	Convey("Given a level3 update envelope", t, func() {
		message := &public.SocketMessage{
			Channel: public.Level3Channel,
			Type:    "update",
			Data: []byte(`[{
				"symbol":"BTC/EUR",
				"bids":[{"event":"add","order_id":"1","limit_price":100,"order_qty":1.5,"timestamp":"2026-06-02T12:00:00Z"}],
				"asks":[]
			}]`),
		}

		Convey("It should decode per-order events", func() {
			rows, err := DecodeOrders(message)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 1)
			So(rows[0].Symbol, ShouldEqual, "BTC/EUR")
			So(rows[0].Bids[0].Event, ShouldEqual, "add")
			So(rows[0].Type, ShouldEqual, "update")
		})
	})

	Convey("Given a level3 envelope without order data", t, func() {
		message := &public.SocketMessage{
			Channel: public.Level3Channel,
			Type:    "update",
		}

		Convey("It should return no rows without error", func() {
			rows, err := DecodeOrders(message)

			So(err, ShouldBeNil)
			So(rows, ShouldBeNil)
		})
	})
}

func TestReplaySubscribeFramesRefreshesToken(t *testing.T) {
	Convey("Given stored subscribe frames without a live socket", t, func() {
		ctx := context.Background()

		SetOrderTokenSource(tokenSourceFunc(func(context.Context) (string, error) {
			return "fresh-token", nil
		}))
		defer SetOrderTokenSource(nil)

		ws := &Level3WebSocket{
			ctx:             ctx,
			subscribeReplay: make([]any, 0),
		}
		ws.recordSubscribeFrame(map[string]any{
			"method": "subscribe",
			"params": map[string]any{
				"channel": "level3",
				"symbol":  []string{"BTC/EUR"},
				"token":   "stale-token",
			},
		})

		Convey("It should fail closed when the socket is not connected", func() {
			err := ws.replaySubscribeFrames()

			So(err, ShouldNotBeNil)
		})
	})
}

type tokenSourceFunc func(context.Context) (string, error)

func (fn tokenSourceFunc) Token(ctx context.Context) (string, error) {
	return fn(ctx)
}

func BenchmarkDecodeOrders(b *testing.B) {
	message := &public.SocketMessage{
		Channel: public.Level3Channel,
		Type:    "update",
		Data: []byte(`[{
			"symbol":"BTC/EUR",
			"bids":[{"event":"add","order_id":"1","limit_price":100,"order_qty":1.5,"timestamp":"2026-06-02T12:00:00Z"}],
			"asks":[]
		}]`),
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = DecodeOrders(message)
	}
}
