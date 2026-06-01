package private

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
)

/*
WebSocket is the authenticated Kraken WebSocket v2 client.
*/
type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	rest        *Rest
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	mu          sync.Mutex
	conns       map[string]*websocket.Conn
	token       string
	tokenUntil  time.Time
}

func NewWebSocket(ctx context.Context, apiKey, apiSecret string) (*WebSocket, error) {
	rest, err := NewRest(ctx, apiKey, apiSecret, public.EndpointWebSocketsToken)

	if err != nil {
		return nil, err
	}

	return NewWebSocketFromRest(ctx, rest)
}

func NewWebSocketFromRest(ctx context.Context, rest *Rest) (*WebSocket, error) {
	if rest == nil {
		return nil, fmt.Errorf("private rest client is required")
	}

	ctx, cancel := context.WithCancel(ctx)

	ws := &WebSocket{
		ctx:    ctx,
		cancel: cancel,
		rest:   rest,
		conns:  make(map[string]*websocket.Conn),
	}

	return ws, errnie.Error(errnie.Require(map[string]any{
		"ctx":    ws.ctx,
		"cancel": ws.cancel,
		"rest":   ws.rest,
		"conns":  ws.conns,
	}))
}

func (ws *WebSocket) Connect(endpoint public.EndpointType, channel string) error {
	if endpoint == "" {
		endpoint = public.WebSocketAuthURL
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()

	if _, ok := ws.conns[channel]; ok {
		return nil
	}

	conn, _, err := websocket.DefaultDialer.Dial(string(endpoint), nil)

	if err != nil {
		return errnie.Error(err)
	}

	ws.conns[channel] = conn

	return nil
}

func (ws *WebSocket) Send(channel string, message any) error {
	token, err := ws.Token(ws.ctx)

	if err != nil {
		return err
	}

	payload, err := sonic.Marshal(message)

	if err != nil {
		return fmt.Errorf("private websocket encode: %w", err)
	}

	var frame map[string]any

	if err := sonic.Unmarshal(payload, &frame); err != nil {
		return fmt.Errorf("private websocket decode: %w", err)
	}

	params, ok := frame["params"].(map[string]any)

	if !ok {
		return fmt.Errorf("private websocket: missing params")
	}

	params["token"] = token

	ws.mu.Lock()
	conn := ws.conns[channel]
	ws.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("channel %s not found", channel)
	}

	return errnie.Error(conn.WriteJSON(frame))
}

func (ws *WebSocket) Tick() error {
	for {
		select {
		case <-ws.ctx.Done():
			return ws.ctx.Err()
		case message := <-ws.subscribers[public.TradesChannel].Incoming:
			if message == nil {
				continue
			}

			ws.broadcasts[message.Channel].Send(&qpool.QValue[any]{Value: message})
		}
	}
}

func (ws *WebSocket) Close(channel string) error {
	ws.mu.Lock()
	conn, ok := ws.conns[channel]

	if !ok {
		ws.mu.Unlock()

		return fmt.Errorf("channel %s not found", channel)
	}

	delete(ws.conns, channel)
	ws.mu.Unlock()

	return errnie.Error(conn.Close())
}

func (ws *WebSocket) Token(ctx context.Context) (string, error) {
	ws.mu.Lock()

	if ws.token != "" && time.Now().Before(ws.tokenUntil.Add(-tokenRefreshLead)) {
		token := ws.token
		ws.mu.Unlock()

		return token, nil
	}

	ws.mu.Unlock()

	token, expires, err := ws.rest.WebSocketToken(ctx)

	if err != nil {
		return "", err
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.token != "" && time.Now().Before(ws.tokenUntil.Add(-tokenRefreshLead)) {
		return ws.token, nil
	}

	ws.token = token
	ws.tokenUntil = time.Now().Add(expires)

	return ws.token, nil
}
