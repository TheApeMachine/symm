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
	"github.com/theapemachine/symm/kraken/ohlc"
)

/*
PrivateClient maintains an authenticated Kraken WebSocket v2 session.
It owns dial, ping, subscribe, and framed reads for public market channels.
*/
type PrivateClient struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	pool          *qpool.Q
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.Subscriber
	conn          *websocket.Conn
	url           string
	once          sync.Once
	reqID         int
	subscriptions []ohlc.Subscribe
}

/*
NewPrivateClient creates a private websocket client bound to parent context cancellation.
*/
func NewPrivateClient(
	ctx context.Context,
	pool *qpool.Q,
	url string,
) *PrivateClient {
	ctx, cancel := context.WithCancel(ctx)

	client := &PrivateClient{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		url:         url,
	}

	client.broadcasts["subscriptions"] = pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)
	client.subscribers["subscriptions"] = client.broadcasts["subscriptions"].Subscribe("subscriptions", 128)

	client.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	client.subscribers["ui"] = client.broadcasts["ui"].Subscribe("ui", 128)

	if errnie.Error(errnie.Require(map[string]any{
		"ctx":         ctx,
		"cancel":      cancel,
		"pool":        pool,
		"broadcasts":  client.broadcasts,
		"subscribers": client.subscribers,
		"url":         url,
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
	case msg := <-privateClient.subscribers["subscriptions"].Incoming:
		if msg, ok := msg.Value.([]string); !ok {
			return errnie.Error(fmt.Errorf("invalid subscriptions message: %v", msg))
		}

		for _, symbol := range msg.Value.([]string) {
			subscription := errnie.Does(func() (*ohlc.Subscribe, error) {
				return ohlc.NewSubscribe([]string{symbol}), nil
			}).Or(func(err error) {
				errnie.Error(err)
			}).Value()

			privateClient.conn.WriteJSON(subscription)
		}

		return nil
	default:
		return nil
	}
}

/*
Connect dials the Kraken v2 websocket endpoint.
Replay sources connect without dialing and require StartReplay after handlers register.
*/
func (privateClient *PrivateClient) Connect() error {
	privateClient.conn, _, privateClient.err = websocket.DefaultDialer.DialContext(
		privateClient.ctx, privateClient.url, nil,
	)

	if privateClient.err != nil {
		return errnie.Error(privateClient.err)
	}

	privateClient.ReadLoop()
	return nil
}

/*
Close cancels the session context and closes the underlying socket.
*/
func (privateClient *PrivateClient) Close() error {
	privateClient.cancel()
	return privateClient.conn.Close()
}

func (privateClient *PrivateClient) ReadLoop() {
	privateClient.once.Do(func() {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-privateClient.ctx.Done():
					return
				case <-ticker.C:
					if err := privateClient.conn.WriteJSON(map[string]string{"method": "ping"}); err != nil {
						errnie.Error(err)
					}
				default:
					_, payload, err := privateClient.conn.ReadMessage()

					if err != nil {
						errnie.Error(err)
						continue
					}

					privateClient.broadcasts["ui"].Send(&qpool.QValue[any]{
						Value: payload,
					})
				}
			}
		}()
	})
}
