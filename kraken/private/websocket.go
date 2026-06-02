package private

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
)

const privateReadPoll = 50 * time.Millisecond

type WebSocket struct {
	ctx             context.Context
	cancel          context.CancelFunc
	err             error
	pool            *qpool.Q
	conn            *websocket.Conn
	broadcasts      map[string]*qpool.BroadcastGroup
	subscribers     map[string]*qpool.Subscriber
	mu              sync.Mutex
	reconnectPolicy *public.ReconnectPolicy
	outboundReplay  []any
	replayMu        sync.RWMutex
	rest            *Rest
	token           string
	tokenUntil      time.Time
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
		ctx:             ctx,
		cancel:          cancel,
		pool:            pool,
		rest:            rest,
		reconnectPolicy: public.NewReconnectPolicyFromConfig(),
		broadcasts:      make(map[string]*qpool.BroadcastGroup),
		subscribers:     make(map[string]*qpool.Subscriber),
		outboundReplay:  make([]any, 0),
	}

	for _, channel := range []string{"raw", "kraken:private"} {
		ws.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		ws.subscribers[channel] = ws.broadcasts[channel].Subscribe(
			"kraken/private:"+channel, 1024,
		)
	}

	if err := user.NewBalance(pool, ws); err != nil {
		errnie.Error(err)
	}

	if err := ws.dialUntilConnected(public.WebSocketAuthURL); err != nil {
		errnie.Error(err)
	} else {
		activate.Boot("kraken/private websocket connected")
	}

	return ws
}

func (ws *WebSocket) Connect(endpoint public.EndpointType, channel string) error {
	if endpoint == "" {
		endpoint = public.WebSocketAuthURL
	}

	ws.mu.Lock()
	connected := ws.conn != nil
	ws.mu.Unlock()

	if connected {
		return nil
	}

	return ws.dialUntilConnected(endpoint)
}

func (ws *WebSocket) Tick() error {
	for {
		select {
		case <-ws.ctx.Done():
			return ws.ctx.Err()
		case message, ok := <-ws.subscribers["kraken:private"].Incoming:
			if !ok {
				errnie.Debug("kraken.private.websocket.Tick", "no ok")
				return ws.ctx.Err()
			}

			if message == nil {
				errnie.Debug("kraken.private.websocket.Tick", "nil message")
				continue
			}

			ws.mu.Lock()
			connected := ws.conn != nil
			ws.mu.Unlock()

			if !connected {
				if reconnectErr := ws.reconnect(public.WebSocketAuthURL); reconnectErr != nil {
					return reconnectErr
				}
			}

			for {
				if err := ws.writeOutboundFrame(message.Value, true); err != nil {
					if ws.ctx.Err() != nil {
						return ws.ctx.Err()
					}

					ws.err = errnie.Error(err)

					if reconnectErr := ws.reconnect(public.WebSocketAuthURL); reconnectErr != nil {
						return reconnectErr
					}

					continue
				}

				break
			}
		default:
		}

		ws.mu.Lock()
		conn := ws.conn
		ws.mu.Unlock()

		if conn == nil {
			if reconnectErr := ws.reconnect(public.WebSocketAuthURL); reconnectErr != nil {
				return reconnectErr
			}

			continue
		}

		if err := conn.SetReadDeadline(time.Now().Add(privateReadPoll)); err != nil {
			ws.err = errnie.Error(err)

			return ws.err
		}

		var message public.SocketMessage

		if err := conn.ReadJSON(&message); err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}

			if ws.ctx.Err() != nil {
				return ws.ctx.Err()
			}

			ws.err = errnie.Error(err)

			if reconnectErr := ws.reconnect(public.WebSocketAuthURL); reconnectErr != nil {
				return reconnectErr
			}

			continue
		}

		_ = conn.SetReadDeadline(time.Time{})

		if message.Channel != "" {
			activate.Once("kraken/private:channel:" + message.Channel)
		}

		if ch := ws.broadcasts["raw"]; ch != nil {
			ch.Send(&qpool.QValue[any]{
				Type:  message.Channel,
				Value: message,
			})
		}

		if message.Channel == public.ExecutionsChannel {
			trading.PublishLedgerAck(ws.pool, message)
		}
	}
}

func (ws *WebSocket) Close() error {
	ws.cancel()
	ws.dropConnection()

	return nil
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
