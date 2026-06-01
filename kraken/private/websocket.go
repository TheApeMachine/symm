package private

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
WebSocket is the authenticated Kraken WebSocket v2 client. It delegates dial,
read, and write to public.WebSocket and injects a session token on every frame.
*/
type WebSocket struct {
	ctx        context.Context
	cancel     context.CancelFunc
	rest       *Rest
	socket     *public.WebSocket
	mu         sync.Mutex
	token      string
	tokenUntil time.Time
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

	socket := errnie.Does(func() (*public.WebSocket, error) {
		return public.NewWebSocket(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	ws := &WebSocket{
		ctx:    ctx,
		cancel: cancel,
		rest:   rest,
		socket: socket,
	}

	return ws, errnie.Error(errnie.Require(map[string]any{
		"ctx":    ws.ctx,
		"cancel": ws.cancel,
		"rest":   ws.rest,
		"socket": ws.socket,
	}))
}

func (ws *WebSocket) Connect(endpoint public.EndpointType, channel string) error {
	if endpoint == "" {
		endpoint = public.WebSocketAuthURL
	}

	return ws.socket.Connect(endpoint, channel)
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

	return ws.socket.Send(channel, frame)
}

func (ws *WebSocket) Stream(channel string) (<-chan *public.SocketMessage, error) {
	return ws.socket.Stream(channel)
}

func (ws *WebSocket) Close(channel string) error {
	return ws.socket.Close(channel)
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
