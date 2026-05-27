package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/core"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/order"
)

/*
PrivateClient maintains an authenticated Kraken WebSocket v2 session.
It forwards private channel frames to ui and routes executions into the pool.
*/
type PrivateClient struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	conn        *websocket.Conn
	url         string
	apiKey      string
	apiSecret   string
	token       string
	once        sync.Once
}

/*
NewPrivateClient creates a private websocket client bound to parent context cancellation.
*/
func NewPrivateClient(
	ctx context.Context,
	pool *qpool.Q,
	url string,
	apiKey string,
	apiSecret string,
) *PrivateClient {
	ctx, cancel := context.WithCancel(ctx)

	client := &PrivateClient{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		url:         url,
		apiKey:      apiKey,
		apiSecret:   apiSecret,
	}

	client.broadcasts["executions"] = pool.CreateBroadcastGroup("executions", 10*time.Millisecond)
	client.subscribers["orders"] = pool.CreateBroadcastGroup("orders", 10*time.Millisecond).Subscribe("private:orders", 128)

	if errnie.Error(errnie.Require(map[string]any{
		"ctx":         ctx,
		"cancel":      cancel,
		"pool":        pool,
		"broadcasts":  client.broadcasts,
		"subscribers": client.subscribers,
		"url":         url,
		"apiKey":      apiKey,
		"apiSecret":   apiSecret,
	})) != nil {
		return nil
	}

	return client
}

func (privateClient *PrivateClient) Start() error {
	return privateClient.Connect()
}

func (privateClient *PrivateClient) State() engine.State {
	return engine.READY
}

func (privateClient *PrivateClient) Tick() error {
	select {
	case <-privateClient.ctx.Done():
		privateClient.cancel()
		return privateClient.ctx.Err()
	case value := <-privateClient.subscribers["orders"].Incoming:
		switch request := value.Value.(type) {
		case order.Request:
			request.Params.Token = privateClient.token

			if err := privateClient.conn.WriteJSON(request); err != nil {
				return errnie.Error(err)
			}
		case order.CancelRequest:
			request.Params.Token = privateClient.token

			if err := privateClient.conn.WriteJSON(request); err != nil {
				return errnie.Error(err)
			}
		default:
			return errnie.Error(fmt.Errorf("invalid order request: %v", value.Value))
		}

		return nil
	default:
		errnie.Warn("this just feels like, spinning plates, system=private client")
		return nil
	}
}

/*
Connect dials the Kraken v2 authenticated websocket endpoint and subscribes to executions.
*/
func (privateClient *PrivateClient) Connect() error {
	if privateClient.apiKey == "" || privateClient.apiSecret == "" {
		return errnie.Error(fmt.Errorf("private client requires API credentials"))
	}

	token, err := kraken.NewToken(privateClient.apiKey, privateClient.apiSecret)

	if err != nil {
		return errnie.Error(err)
	}

	privateClient.token = token.Value()

	privateClient.conn, _, privateClient.err = websocket.DefaultDialer.DialContext(
		privateClient.ctx, privateClient.url, nil,
	)

	if privateClient.err != nil {
		return errnie.Error(privateClient.err)
	}

	if err := privateClient.conn.WriteJSON(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":     core.ChannelExecutions,
			"token":       privateClient.token,
			"snap_orders": true,
		},
	}); err != nil {
		return errnie.Error(err)
	}

	privateClient.ReadLoop()
	return nil
}

/*
Close cancels the session context and closes the underlying socket.
*/
func (privateClient *PrivateClient) Close() error {
	privateClient.cancel()

	if privateClient.conn == nil {
		return nil
	}

	return privateClient.conn.Close()
}

func (privateClient *PrivateClient) ReadLoop() {
	privateClient.once.Do(func() {
		go func() {
			pingStop := make(chan struct{})
			defer close(pingStop)

			go func() {
				ticker := time.NewTicker(30 * time.Second)
				defer ticker.Stop()

				for {
					select {
					case <-privateClient.ctx.Done():
						return
					case <-pingStop:
						return
					case <-ticker.C:
						if privateClient.conn == nil {
							return
						}

						if err := privateClient.conn.WriteJSON(map[string]string{"method": "ping"}); err != nil {
							return
						}
					}
				}
			}()

			for privateClient.ctx.Err() == nil {
				_, payload, err := privateClient.conn.ReadMessage()

				if err != nil {
					errnie.Error(err)
					return
				}

				channel, err := market.ChannelName(payload)

				if err != nil {
					continue
				}

				if channel != core.ChannelExecutions {
					continue
				}

				fills, err := order.ParseExecutionFills(payload)

				if err != nil {
					errnie.Error(err)
					continue
				}

				executions := privateClient.broadcasts["executions"]

				for _, fill := range fills {
					executions.Send(&qpool.QValue[any]{Value: fill})
				}
			}
		}()
	})
}
