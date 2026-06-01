package private

import (
	"context"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
)

type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	conn        *websocket.Conn
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	mu          sync.Mutex
	rest        *Rest
	token       string
	tokenUntil  time.Time
}

func NewWebSocket(ctx context.Context, pool *qpool.Q, apiKey, apiSecret string) (*WebSocket, error) {
	rest, err := NewRest(ctx, apiKey, apiSecret, public.EndpointWebSocketsToken)

	if err != nil {
		return nil, err
	}

	return NewWebSocketFromRest(ctx, pool, rest)
}

func NewWebSocketFromRest(ctx context.Context, pool *qpool.Q, rest *Rest) (*WebSocket, error) {
	ctx, cancel := context.WithCancel(ctx)

	ws := &WebSocket{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		rest:        rest,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
	}

	for _, channel := range []string{"executions", "balances", "orders"} {
		ws.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		ws.subscribers[channel] = ws.broadcasts[channel].Subscribe(channel, 128)
	}

	return ws, errnie.Error(errnie.Require(map[string]any{
		"ctx":    ws.ctx,
		"cancel": ws.cancel,
		"rest":   ws.rest,
	}))
}

func (ws *WebSocket) Connect(endpoint public.EndpointType, channel string) error {
	if endpoint == "" {
		endpoint = public.WebSocketAuthURL
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.conn != nil {
		return nil
	}

	conn, _, err := websocket.DefaultDialer.Dial(string(endpoint), nil)

	if err != nil {
		return errnie.Error(err)
	}

	ws.conn = conn

	return nil
}

func (ws *WebSocket) Tick() error {
	incoming := make(chan public.SocketMessage, 64)
	readerErr := make(chan error, 1)

	go ws.readLoop(incoming, readerErr)

	ordersIncoming := ws.subscribers["orders"].Incoming

	for {
		select {
		case <-ws.ctx.Done():
			return ws.ctx.Err()
		case err := <-readerErr:
			if err != nil {
				return err
			}

			return ws.ctx.Err()
		case message, ok := <-ordersIncoming:
			if !ok {
				return ws.ctx.Err()
			}

			if message == nil {
				continue
			}

			ws.mu.Lock()
			conn := ws.conn
			ws.mu.Unlock()

			if conn == nil {
				continue
			}

			if err := conn.WriteJSON(message.Value); err != nil {
				errnie.Error(err)
			}
		case socketMessage := <-incoming:
			if ch := ws.broadcasts[socketMessage.Channel]; ch != nil {
				ch.Send(&qpool.QValue[any]{Value: socketMessage})
			}
		}
	}
}

func (ws *WebSocket) readLoop(
	out chan<- public.SocketMessage,
	errOut chan<- error,
) {
	defer close(out)

	for {
		select {
		case <-ws.ctx.Done():
			errOut <- ws.ctx.Err()
			return
		default:
		}

		ws.mu.Lock()
		conn := ws.conn
		ws.mu.Unlock()

		if conn == nil {
			continue
		}

		var message public.SocketMessage

		if err := conn.ReadJSON(&message); err != nil {
			errOut <- err
			return
		}

		select {
		case out <- message:
		case <-ws.ctx.Done():
			errOut <- ws.ctx.Err()
			return
		}
	}
}

func (ws *WebSocket) Close() error {
	ws.cancel()

	ws.mu.Lock()
	conn := ws.conn
	ws.mu.Unlock()

	if conn == nil {
		return nil
	}

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

	ws.token = token
	ws.tokenUntil = time.Now().Add(expires)

	return ws.token, nil
}
