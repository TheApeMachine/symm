package replay_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	krakenreplay "github.com/theapemachine/symm/kraken/replay"
)

func TestSocketMessageSplitDataRows(t *testing.T) {
	convey.Convey("Given a recorded book frame", t, func() {
		payload := []byte(`{"channel":"book","type":"snapshot","data":[{"symbol":"BTC/EUR","bids":[{"price":49990,"qty":1}],"asks":[{"price":50010,"qty":1}]}]}`)

		var envelope public.SocketMessage

		err := sonic.Unmarshal(payload, &envelope)
		convey.So(err, convey.ShouldBeNil)

		messages, err := envelope.SplitDataRows()

		convey.Convey("It should split data rows with envelope type", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(messages), convey.ShouldEqual, 1)
			convey.So(messages[0].Type, convey.ShouldEqual, "snapshot")
			convey.So(string(messages[0].Data), convey.ShouldContainSubstring, `"symbol":"BTC/EUR"`)
		})
	})
}

func TestWebSocketSendOrdersThroughPaper(t *testing.T) {
	convey.Convey("Given a replay websocket", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws, err := krakenreplay.NewWebSocket(ctx, pool, strings.NewReader(""))

		convey.So(err, convey.ShouldBeNil)

		err = ws.Connect(public.WebSocketAuthURL, public.ExecutionsChannel)
		convey.So(err, convey.ShouldBeNil)

		subscriber := pool.CreateBroadcastGroup(public.ExecutionsChannel, 10*time.Millisecond).
			Subscribe("test", 128)

		err = ws.Send(public.OrdersChannel, map[string]any{
			"method": "add_order",
			"params": map[string]any{
				"order_type":  "limit",
				"side":        "buy",
				"symbol":      "BTC/EUR",
				"order_qty":   0.01,
				"limit_price": 50000.0,
				"cl_ord_id":   "replay-paper-001",
			},
		})

		convey.Convey("It should route add_order through paper execution", func() {
			convey.So(err, convey.ShouldBeNil)

			message, ok := <-subscriber.Incoming
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(message, convey.ShouldNotBeNil)

			envelope, ok := message.Value.(public.SocketMessage)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(envelope.Channel, convey.ShouldEqual, public.ExecutionsChannel)
		})
	})
}

func TestWebSocketTickReplayCapture(t *testing.T) {
	convey.Convey("Given a replay capture", t, func() {
		path := filepath.Join(t.TempDir(), "capture.jsonl")
		line := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/EUR","side":"buy","price":1,"qty":1,"ord_type":"market","trade_id":1,"timestamp":"2026-05-31T00:00:00Z"}]}` + "\n")
		err := os.WriteFile(path, line, 0o644)
		convey.So(err, convey.ShouldBeNil)

		file, err := os.Open(path)
		convey.So(err, convey.ShouldBeNil)

		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws, err := krakenreplay.NewWebSocket(ctx, pool, file)
		convey.So(err, convey.ShouldBeNil)

		subscriber := pool.CreateBroadcastGroup(public.TradesChannel, 10*time.Millisecond).
			Subscribe("test", 128)

		go func() {
			_ = ws.Tick()
		}()

		message, ok := <-subscriber.Incoming

		convey.Convey("It should broadcast trade frames", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(message, convey.ShouldNotBeNil)

			envelope, ok := message.Value.(public.SocketMessage)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(envelope.Type, convey.ShouldEqual, "update")
			convey.So(string(envelope.Data), convey.ShouldContainSubstring, `"symbol":"BTC/EUR"`)
		})
	})
}

func BenchmarkSocketMessageSplitDataRows(b *testing.B) {
	payload := []byte(`{"channel":"book","type":"snapshot","data":[{"symbol":"BTC/EUR","bids":[{"price":49990,"qty":1}],"asks":[{"price":50010,"qty":1}]}]}`)

	var envelope public.SocketMessage

	_ = sonic.Unmarshal(payload, &envelope)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = envelope.SplitDataRows()
	}
}
