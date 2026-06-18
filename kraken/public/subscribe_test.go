package public

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
)

func TestWebSocketSubscribeMarket(t *testing.T) {
	viper.Set("system.network.connection.max_delay", 89)
	viper.Set("market.subscribe_pace", 0)
	viper.Set("market.subscribe_batch", 2)
	viper.Set("market.book_depth_levels", 10)

	Convey("Given a public websocket with discovered symbols", t, func() {
		pool := websocketTestPool(t)
		tree := websocketTestTree(t)
		ws := NewWebSocket(t.Context(), pool, tree)
		ws.SetSymbols([]string{"BTC/USD", "ETH/USD", "SOL/USD"})

		received := make(chan []byte, 8)

		pool.Subscribe("kraken:public", func(artifact *datura.Artifact) error {
			payload, payloadErr := artifact.Payload()

			if payloadErr == nil {
				received <- append([]byte(nil), payload...)
			}

			return nil
		})

		Convey("When subscribeMarket is called", func() {
			err := ws.subscribeMarket()

			Convey("It should publish subscribe frames to kraken:public", func() {
				So(err, ShouldBeNil)

				frames := collectSubscribeFrames(received, 7)

				So(frames, ShouldHaveLength, 7)
				So(frames[0], shouldSubscribeChannel, "instrument")
				So(frames[1], shouldSubscribeChannel, "book")
				So(frames[2], shouldSubscribeChannel, "trade")
				So(frames[3], shouldSubscribeChannel, "ticker")
				So(frames[4], shouldSubscribeChannel, "book")
			})
		})

		Convey("When a subscribe artifact is routed through kraken:public", func() {
			message, buildErr := types.NewKrakenMessage("subscribe", market.TickerParams{
				Channel:  "ticker",
				Symbol:   []string{"BTC/USD"},
				Snapshot: true,
			}, 0)

			So(buildErr, ShouldBeNil)

			payload, marshalErr := sonic.Marshal(message)

			So(marshalErr, ShouldBeNil)

			wire := make(chan []byte, 1)

			Convey("It should write the payload to the websocket", WithConnectedWebSocket(t, ws, func(conn *websocket.Conn) {
				messageType, message, readErr := conn.ReadMessage()

				if readErr == nil && messageType == websocket.TextMessage {
					wire <- append([]byte(nil), message...)
				}

				holdServerConn(conn)
			}, func() {
				artifact := datura.Acquire("public", datura.Artifact_Type_json).
					WithDestination("kraken:public").
					WithPayload(payload)

				defer artifact.Release()

				err := pool.CreateBroadcastGroup("kraken:public").Send(artifact)

				So(err, ShouldBeNil)

				var frame []byte

				select {
				case frame = <-wire:
				case <-time.After(2 * time.Second):
					So("kraken:public payload", ShouldEqual, "written")
				}

				So(string(frame), ShouldEqual, string(payload))
			}))
		})
	})
}

func TestWebSocketSubscribeMarketFailure(t *testing.T) {
	viper.Set("system.network.connection.max_delay", 89)
	viper.Set("market.subscribe_pace", 0)
	viper.Set("market.subscribe_batch", 2)
	viper.Set("market.book_depth_levels", 10)

	Convey("Given a public websocket with a closed broadcast group", t, func() {
		pool := websocketTestPool(t)
		tree := websocketTestTree(t)
		ws := NewWebSocket(t.Context(), pool, tree)
		ws.SetSymbols([]string{"BTC/USD"})

		So(pool.CreateBroadcastGroup("kraken:public").Close(), ShouldBeNil)

		Convey("When subscribeMarket is called", func() {
			err := ws.subscribeMarket()

			Convey("It should return an error and leave subscribed unset", func() {
				So(err, ShouldNotBeNil)
				So(ws.subscribed.Load(), ShouldBeFalse)
			})
		})
	})
}

func collectSubscribeFrames(received <-chan []byte, count int) [][]byte {
	frames := make([][]byte, 0, count)
	deadline := time.After(2 * time.Second)

	for len(frames) < count {
		select {
		case frame := <-received:
			frames = append(frames, frame)
		case <-deadline:
			return frames
		}
	}

	return frames
}

func shouldSubscribeChannel(actual any, expected ...any) string {
	frame, ok := actual.([]byte)

	if !ok {
		return "payload should be bytes"
	}

	message := struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}{}

	if err := sonic.Unmarshal(frame, &message); err != nil {
		return err.Error()
	}

	params := map[string]any{}

	if err := sonic.Unmarshal(message.Params, &params); err != nil {
		return err.Error()
	}

	channel, ok := params["channel"].(string)

	if !ok {
		return "channel missing"
	}

	if channel != expected[0].(string) {
		return "unexpected channel"
	}

	return ""
}
