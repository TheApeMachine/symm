package public

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

var socket *WebSocket
var socketOnce sync.Once

type SocketMessage struct {
	Channel string          `json:"channel"`
	Type    string          `json:"type"`
	Data    json.RawMessage `json:"data"`
}

type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q
	conn        *websocket.Conn
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	recorder    io.Writer
}

func NewWebSocket(ctx context.Context, pool *qpool.Q) (*WebSocket, error) {
	ctx, cancel := context.WithCancel(ctx)

	socketOnce.Do(func() {
		socket = &WebSocket{
			ctx:         ctx,
			cancel:      cancel,
			pool:        pool,
			broadcasts:  make(map[string]*qpool.BroadcastGroup),
			subscribers: make(map[string]*qpool.Subscriber),
		}
	})

	for _, channel := range []string{
		"kraken:public", "ticker", "book", "trade", "ohlc", "instrument", "level3",
	} {
		socket.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		socket.subscribers[channel] = socket.broadcasts[channel].Subscribe(channel, 128)
	}

	if viper.GetViper().Get("trading.model") == "record" {
		recorder, err := os.Create(viper.GetViper().GetString("trading.record.file"))

		if err != nil {
			return nil, err
		}

		socket.recorder = bufio.NewWriter(recorder)
	}

	return socket, errnie.Error(errnie.Require(map[string]any{
		"ctx":    socket.ctx,
		"cancel": socket.cancel,
		"pool":   socket.pool,
	}))
}

func (ws *WebSocket) Connect(endpoint EndpointType, channel string) error {
	errnie.Debug("kraken.public.websocket.Connect", endpoint, channel)

	if endpoint == "" {
		endpoint = WebSocketURL
	}

	if ws.conn, _, ws.err = websocket.DefaultDialer.Dial(
		string(endpoint), nil,
	); ws.err != nil {
		return ws.err
	}

	return nil
}

func (ws *WebSocket) Tick() error {
	for {
		select {
		case <-ws.ctx.Done():
			return ws.err
		case message, ok := <-ws.subscribers["kraken:public"].Incoming:
			if !ok {
				return nil
			}

			if message == nil {
				continue
			}

			ws.conn.WriteJSON(message.Value)
		default:
		}

		var message SocketMessage

		if err := ws.conn.ReadJSON(&message); err != nil {
			return err
		}

		if ws.recorder != nil {
			ws.recorder.Write(append(message.Data, []byte("\n")...))
		}

		ws.broadcasts[message.Channel].Send(&qpool.QValue[any]{Value: message})
	}
}

func (ws *WebSocket) Close() error {
	ws.cancel()

	if ws.conn == nil {
		return nil
	}

	return errnie.Error(ws.conn.Close())
}
