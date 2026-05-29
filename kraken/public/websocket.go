package public

import (
	"context"
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
)

type WebSocket struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	conns  map[string]*websocket.Conn
}

func NewWebSocket(ctx context.Context) (*WebSocket, error) {
	ctx, cancel := context.WithCancel(ctx)

	ws := &WebSocket{
		ctx:    ctx,
		cancel: cancel,
		conns:  make(map[string]*websocket.Conn),
	}

	return ws, errnie.Error(errnie.Require(map[string]any{
		"ctx":    ctx,
		"cancel": cancel,
		"conns":  ws.conns,
	}))
}

func (ws *WebSocket) Connect(endpoint EndpointType, channel string) error {
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

	return errnie.Error(conn.WriteJSON(message))
}

func (ws *WebSocket) Generate(channel string) (chan *SocketMessage, error) {
	conn, ok := ws.conns[channel]

	if !ok {
		return nil, fmt.Errorf("channel %s not found", channel)
	}

	out := make(chan *SocketMessage)

	go func() {
		defer close(out)

		for {
			select {
			case <-ws.ctx.Done():
				return
			default:
				_, payload, err := conn.ReadMessage()

				if err != nil {
					return
				}

				var message SocketMessage

				if err := sonic.Unmarshal(payload, &message); err != nil {
					continue
				}

				if message.Channel == "" {
					continue
				}

				out <- &message
			}
		}
	}()

	return out, nil
}

func (ws *WebSocket) Close(channel string) error {
	conn, ok := ws.conns[channel]

	if !ok {
		return fmt.Errorf("channel %s not found", channel)
	}

	return errnie.Error(conn.Close())
}
