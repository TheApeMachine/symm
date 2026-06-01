package public

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

var socket *WebSocket
var socketOnce sync.Once

type WebSocketClient interface {
	Connect(endpoint EndpointType, channel string) error
	Send(channel string, message any) error
	Stream(channel string) (<-chan *SocketMessage, error)
	Close(channel string) error
}

type WebSocket struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	conns    map[string]*websocket.Conn
	recorder io.Writer
}

func NewWebSocket(ctx context.Context) (*WebSocket, error) {
	ctx, cancel := context.WithCancel(ctx)

	socketOnce.Do(func() {
		socket = &WebSocket{
			ctx:    ctx,
			cancel: cancel,
			conns:  make(map[string]*websocket.Conn),
		}
		socket.bindRecorder()
	})

	return socket, errnie.Error(errnie.Require(map[string]any{
		"ctx":    socket.ctx,
		"cancel": socket.cancel,
		"conns":  socket.conns,
	}))
}

func (ws *WebSocket) bindRecorder() {
	if viper.GetViper().Get("trading.model") != "record" {
		return
	}

	recorder, err := os.Create(viper.GetViper().GetString("trading.record.file"))

	if err != nil {
		errnie.Error(err)

		return
	}

	ws.recorder = recorder
}

func (ws *WebSocket) Connect(endpoint EndpointType, channel string) error {
	errnie.Debug("kraken.public.websocket.Connect", endpoint, channel)

	if endpoint == "" {
		endpoint = WebSocketURL
	}

	conn, _, err := websocket.DefaultDialer.Dial(string(endpoint), nil)

	if err != nil {
		return errnie.Error(err)
	}

	ws.conns[channel] = conn

	return nil
}

func (ws *WebSocket) Send(channel string, message any) error {
	conn, ok := ws.conns[channel]

	if !ok {
		return fmt.Errorf("channel %s not found", channel)
	}

	if err := conn.WriteJSON(message); err != nil {
		return errnie.Error(err)
	}

	return nil
}

func (ws *WebSocket) Stream(channel string) (<-chan *SocketMessage, error) {
	conn, ok := ws.conns[channel]

	if !ok {
		return nil, fmt.Errorf("channel %s not found", channel)
	}

	out := make(chan *SocketMessage, 256)

	go func() {
		defer close(out)

		for {
			select {
			case <-ws.ctx.Done():
				return
			default:
				message, ok := ws.read(conn, channel)

				if !ok || message == nil {
					continue
				}

				ws.record(message)

				if err := message.EmitRows(ws.ctx, out); err != nil {
					errnie.Error(err)
				}
			}
		}
	}()

	return out, nil
}

func (ws *WebSocket) StreamSnapshot(channel string) (<-chan *SocketMessage, error) {
	conn, ok := ws.conns[channel]

	if !ok {
		return nil, fmt.Errorf("channel %s not found", channel)
	}

	out := make(chan *SocketMessage, 256)

	go func() {
		defer close(out)

		for {
			select {
			case <-ws.ctx.Done():
				return
			default:
				message, ok := ws.read(conn, channel)

				if !ok {
					return
				}

				if message == nil {
					continue
				}

				select {
				case <-ws.ctx.Done():
					return
				case out <- message:
				}
			}
		}
	}()

	return out, nil
}

func (ws *WebSocket) read(
	conn *websocket.Conn, channel string,
) (*SocketMessage, bool) {
	_, payload, err := conn.ReadMessage()

	if err != nil {
		return nil, false
	}

	var message SocketMessage

	if err := sonic.Unmarshal(payload, &message); err != nil {
		errnie.Error(fmt.Errorf("kraken ws envelope decode %s: %w", channel, err))

		return nil, true
	}

	if message.Channel != channel || len(message.Data) == 0 {
		return nil, true
	}

	return &message, true
}

func (ws *WebSocket) record(message *SocketMessage) {
	if ws.recorder == nil {
		return
	}

	ws.recorder.Write(message.Data)
}

func (ws *WebSocket) Close(channel string) error {
	conn, ok := ws.conns[channel]

	if !ok {
		return fmt.Errorf("channel %s not found", channel)
	}

	return errnie.Error(conn.Close())
}
