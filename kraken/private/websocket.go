package private

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/symm/kraken/public"
)

const (
	reconnectBackoff  = 2 * time.Second
	maxReconnectPause = 30 * time.Second
)

/*
MessageHandler receives one raw WebSocket frame from the authenticated v2 API.
*/
type MessageHandler func(payload []byte)

/*
WebSocket is the authenticated Kraken WebSocket v2 connection for private
channels and trading methods. Token refresh uses the paired Rest client.
*/
type WebSocket struct {
	ctx        context.Context
	cancel     context.CancelFunc
	rest       *Rest
	conn       *websocket.Conn
	token      string
	tokenUntil time.Time
	reqID      atomic.Uint64
	handler    MessageHandler
	mu         sync.Mutex
	writeMu    sync.Mutex
}

/*
NewWebSocket wires private REST credentials for token refresh and frame routing.
*/
func NewWebSocket(ctx context.Context, apiKey, apiSecret string) (*WebSocket, error) {
	rest, err := NewRest(ctx, apiKey, apiSecret, EndpointWebSocketsToken)

	if err != nil {
		return nil, err
	}

	return NewWebSocketFromRest(ctx, rest)
}

/*
NewWebSocketFromRest builds an authenticated socket from an existing Rest client.
*/
func NewWebSocketFromRest(ctx context.Context, rest *Rest) (*WebSocket, error) {
	if rest == nil {
		return nil, fmt.Errorf("private rest client is required")
	}

	ctx, cancel := context.WithCancel(ctx)

	return &WebSocket{
		ctx:    ctx,
		cancel: cancel,
		rest:   rest,
	}, nil
}

/*
OnMessage registers the handler invoked for every inbound frame.
*/
func (socket *WebSocket) OnMessage(handler MessageHandler) {
	socket.handler = handler
}

/*
Rest returns the REST client used for token refresh.
*/
func (socket *WebSocket) Rest() *Rest {
	return socket.rest
}

/*
Context returns the socket lifecycle context.
*/
func (socket *WebSocket) Context() context.Context {
	return socket.ctx
}

/*
Start dials the auth socket, subscribes to executions, and begins reading frames.
*/
func (socket *WebSocket) Start() error {
	if err := socket.connect(); err != nil {
		return err
	}

	go socket.readLoop()

	return nil
}

/*
Token returns a valid WebSocket token, refreshing when near expiry.
*/
func (socket *WebSocket) Token(ctx context.Context) (string, error) {
	if err := socket.refreshToken(); err != nil {
		return "", err
	}

	socket.mu.Lock()
	defer socket.mu.Unlock()

	return socket.token, nil
}

/*
NextReqID returns the next monotonic request id for trading method frames.
*/
func (socket *WebSocket) NextReqID() int {
	return int(socket.reqID.Add(1))
}

/*
WriteJSON sends one frame on the authenticated socket.
*/
func (socket *WebSocket) WriteJSON(payload any) error {
	socket.mu.Lock()
	conn := socket.conn
	socket.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("authenticated websocket is not connected")
	}

	socket.writeMu.Lock()
	defer socket.writeMu.Unlock()

	return conn.WriteJSON(payload)
}

/*
Close shuts down the socket and returns any connection close error.
*/
func (socket *WebSocket) Close() error {
	socket.cancel()

	socket.mu.Lock()
	defer socket.mu.Unlock()

	if socket.conn == nil {
		return nil
	}

	closeErr := socket.conn.Close()
	socket.conn = nil

	return closeErr
}

func (socket *WebSocket) refreshToken() error {
	socket.mu.Lock()

	if socket.token != "" && time.Now().Before(socket.tokenUntil.Add(-tokenRefreshLead)) {
		socket.mu.Unlock()

		return nil
	}

	socket.mu.Unlock()

	token, expires, err := socket.rest.WebSocketToken(socket.ctx)

	if err != nil {
		return err
	}

	socket.mu.Lock()
	defer socket.mu.Unlock()

	if socket.token != "" && time.Now().Before(socket.tokenUntil.Add(-tokenRefreshLead)) {
		return nil
	}

	socket.token = token
	socket.tokenUntil = time.Now().Add(expires)

	return nil
}

func (socket *WebSocket) connect() error {
	if err := socket.refreshToken(); err != nil {
		return err
	}

	conn, _, err := websocket.DefaultDialer.Dial(string(public.WebSocketAuthURL), nil)

	if err != nil {
		return fmt.Errorf("dial kraken auth websocket: %w", err)
	}

	socket.mu.Lock()
	socket.conn = conn
	token := socket.token
	socket.mu.Unlock()

	if err := socket.WriteJSON(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": "executions",
			"token":   token,
		},
	}); err != nil {
		socket.closeConn()

		return fmt.Errorf("subscribe executions: %w", err)
	}

	return nil
}

func (socket *WebSocket) closeConn() {
	socket.mu.Lock()
	defer socket.mu.Unlock()

	if socket.conn == nil {
		return
	}

	_ = socket.conn.Close()
	socket.conn = nil
}

func (socket *WebSocket) readLoop() {
	backoff := reconnectBackoff

	for {
		if socket.ctx.Err() != nil {
			return
		}

		socket.mu.Lock()
		conn := socket.conn
		socket.mu.Unlock()

		if conn == nil {
			if err := socket.reconnectWithBackoff(&backoff); err != nil {
				return
			}

			continue
		}

		_, payload, err := conn.ReadMessage()

		if err != nil {
			socket.closeConn()

			if socket.ctx.Err() != nil {
				return
			}

			if reconnectErr := socket.reconnectWithBackoff(&backoff); reconnectErr != nil {
				return
			}

			continue
		}

		backoff = reconnectBackoff

		if socket.handler != nil {
			socket.handler(payload)
		}
	}
}

func (socket *WebSocket) reconnectWithBackoff(backoff *time.Duration) error {
	for socket.ctx.Err() == nil {
		if err := socket.connect(); err != nil {
			select {
			case <-socket.ctx.Done():
				return socket.ctx.Err()
			case <-time.After(*backoff):
			}

			if *backoff < maxReconnectPause {
				*backoff *= 2

				if *backoff > maxReconnectPause {
					*backoff = maxReconnectPause
				}
			}

			continue
		}

		*backoff = reconnectBackoff

		return nil
	}

	return socket.ctx.Err()
}
