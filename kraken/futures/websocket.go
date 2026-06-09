package futures

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
)

var socket *WebSocket
var socketOnce sync.Once

type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q[any]
	conn        *websocket.Conn
	bus         *internal.Bus
	isConnected atomic.Bool
}

func NewWebSocket(ctx context.Context, pool *qpool.Q[any]) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	socketOnce.Do(func() {
		socket = &WebSocket{
			ctx:    ctx,
			cancel: cancel,
			pool:   pool,
			bus: internal.NewBus(
				ctx,
				pool,
				[]string{"raw", "kraken:futures"},
				[]string{"kraken:futures"},
			),
		}

		errnie.Info("kraken/futures websocket ready")
	})

	return socket
}

func (ws *WebSocket) Connect(retry uint64) error {
	if ws.isConnected.Load() && ws.conn != nil {
		return nil
	}

	ws.isConnected.Store(false)

	var response *http.Response

	conn, response, dialErr := websocket.DefaultDialer.Dial(string(WebSocketURL), nil)

	if dialErr != nil {
		if response != nil {
			errnie.Error(dialErr, response.StatusCode, response.Status)
		}

		retry = uint64(
			math.Round((math.Pow(
				math.Phi, float64(retry),
			) + math.Pow(
				math.Phi-1, float64(retry),
			)) / math.Sqrt(5)),
		)

		time.Sleep(time.Duration(retry) * time.Second)

		return ws.Connect(retry)
	}

	ws.conn = conn
	ws.isConnected.Store(true)

	return nil
}

func (ws *WebSocket) disconnect() {
	ws.isConnected.Store(false)

	if ws.conn == nil {
		return
	}

	errnie.Error(ws.conn.Close())
	ws.conn = nil
}

func (ws *WebSocket) Tick() error {
	pingInterval := viper.GetDuration("market.futures_ws_ping_interval")

	if pingInterval <= 0 {
		pingInterval = 30 * time.Second
	}

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		if err := ws.Connect(0); err != nil {
			continue
		}

		select {
		case <-ws.ctx.Done():
			return ws.ctx.Err()
		case <-ticker.C:
			if ws.conn == nil {
				ws.disconnect()
				break
			}

			if errnie.Error(ws.conn.WriteJSON(PingMessage{Event: "ping"})) != nil {
				ws.disconnect()
			}
		default:
			message, pollErr := ws.bus.Poll("kraken:futures")

			if errnie.Error(pollErr) != nil || message == nil {
				break
			}

			if errnie.Error(ws.conn.WriteJSON(message.Value)) != nil {
				ws.disconnect()
				continue
			}
		}

		if !ws.isConnected.Load() || ws.conn == nil {
			ws.disconnect()
			continue
		}

		_, payload, readErr := ws.conn.ReadMessage()

		if readErr != nil {
			ws.disconnect()
			continue
		}

		if handleErr := ws.handleMessage(payload); handleErr != nil {
			errnie.Error(handleErr)
		}
	}
}

func (ws *WebSocket) handleMessage(payload []byte) error {
	var envelope struct {
		Event     string `json:"event"`
		Feed      string `json:"feed"`
		Message   string `json:"message"`
		ProductID string `json:"product_id"`
	}

	if err := json.Unmarshal(payload, &envelope); err != nil {
		return err
	}

	if envelope.Event == "pong" || envelope.Event == "heartbeat" {
		return nil
	}

	if envelope.Event == "error" {
		return errors.New(envelope.Message)
	}

	switch envelope.Feed {
	case bookSnapshotFeed:
		var snapshot bookSnapshotMessage

		if err := json.Unmarshal(payload, &snapshot); err != nil {
			return err
		}

		book, err := BookFromSnapshot(snapshot)

		if err != nil {
			return err
		}

		ws.bus.Send("raw", "futures_book", &book)

	case bookDeltaFeed:
		var delta bookDeltaMessage

		if err := json.Unmarshal(payload, &delta); err != nil {
			return err
		}

		book, err := BookFromDelta(delta)

		if err != nil {
			return err
		}

		ws.bus.Send("raw", "futures_book", &book)
	}

	return nil
}

func (ws *WebSocket) Close() error {
	ws.disconnect()
	ws.cancel()

	return nil
}
