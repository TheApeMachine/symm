package private

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/kraken/public"
)

type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q
	conn        *websocket.Conn
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	mu          sync.Mutex
	rest        *Rest
	token       string
	tokenUntil  time.Time
}

func NewWebSocket(
	ctx context.Context, pool *qpool.Q, apiKey, apiSecret string,
) public.WebSocketClient {
	if viper.GetViper().GetString("trading.model") == "paper" {
		activate.Boot("kraken/private paper websocket")

		return paper.NewWebSocket(ctx, pool)
	}

	activate.Boot("kraken/private live websocket")

	rest, err := NewRest(
		ctx, apiKey, apiSecret, public.EndpointWebSocketsToken,
	)

	if err != nil {
		return nil
	}

	return NewWebSocketFromRest(ctx, pool, rest)
}

func NewWebSocketFromRest(
	ctx context.Context, pool *qpool.Q, rest *Rest,
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	ws := &WebSocket{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		rest:        rest,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
	}

	for _, channel := range []string{"raw", "kraken:private"} {
		ws.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		ws.subscribers[channel] = ws.broadcasts[channel].Subscribe(
			"kraken/private:"+channel, 128,
		)
	}

	return ws
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

	activate.Once("kraken/private:connected:" + string(endpoint))

	return nil
}

func (ws *WebSocket) Tick() error {
	for {
		select {
		case <-ws.ctx.Done():
			return ws.err
		case message, ok := <-ws.subscribers["kraken:private"].Incoming:
			if !ok {
				errnie.Debug("kraken.private.websocket.Tick", "no ok")
				return nil
			}

			if message == nil {
				errnie.Debug("kraken.private.websocket.Tick", "nil message")
				continue
			}

			if ws.conn == nil {
				return fmt.Errorf("kraken private websocket: not connected")
			}

			errnie.Error(ws.conn.WriteJSON(message.Value))
		default:
		}

		if ws.conn == nil {
			return fmt.Errorf("kraken private websocket: not connected")
		}

		var message public.SocketMessage

		if err := ws.conn.ReadJSON(&message); err != nil {
			return errnie.Error(err)
		}

		if message.Channel != "" {
			activate.Once("kraken/private:channel:" + message.Channel)
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
