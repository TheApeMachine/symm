package public

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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

const publicOutboundSubscriberID = "kraken:public:websocket"

type WebSocketClient interface {
	Connect(endpoint EndpointType, channel string) error
	Tick() error
	Close() error
}

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
	pairs       []string
}

func NewWebSocket(ctx context.Context, pool *qpool.Q) *WebSocket {
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
		"raw", "level3",
	} {
		socket.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		socket.subscribers[channel] = socket.broadcasts[channel].Subscribe(channel, 128)
	}

	socket.broadcasts["kraken:public"] = pool.CreateBroadcastGroup("kraken:public", 10*time.Millisecond)
	socket.subscribers["kraken:public"] = socket.broadcasts["kraken:public"].Subscribe(
		publicOutboundSubscriberID, 128,
	)

	if viper.GetViper().Get("trading.model") == "record" {
		recorder, err := os.Create(viper.GetViper().GetString("trading.record.file"))

		if err != nil {
			errnie.Error(err)
		}

		socket.recorder = bufio.NewWriter(recorder)
	}

	if err := socket.Connect(WebSocketURL, "kraken:public"); err != nil {
		errnie.Error(err)
	}

	errnie.Debug("kraken.public.websocket.NewWebSocket", "subscribing to", "instrument")

	socket.broadcasts["kraken:public"].Send(&qpool.QValue[any]{Value: map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":  "instrument",
			"snapshot": true,
		},
	}})

	return socket
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
				errnie.Debug("kraken.public.websocket.Tick", "no ok")
				return nil
			}

			if message == nil {
				errnie.Debug("kraken.public.websocket.Tick", "nil message")
				continue
			}

			if ws.conn == nil {
				return fmt.Errorf("kraken public websocket: not connected")
			}

			errnie.Error(ws.conn.WriteJSON(message.Value))
		default:
		}

		if ws.conn == nil {
			return fmt.Errorf("kraken public websocket: not connected")
		}

		var message SocketMessage

		if err := ws.conn.ReadJSON(&message); err != nil {
			return errnie.Error(err)
		}

		if ch := ws.broadcasts["raw"]; ch != nil {
			ch.Send(&qpool.QValue[any]{
				Type:  message.Channel,
				Value: message,
			})
		}
	}
}

func (ws *WebSocket) Close() error {
	ws.cancel()

	if ws.conn == nil {
		return nil
	}

	return errnie.Error(ws.conn.Close())
}
