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
PublicClient maintains an unauthenticated Kraken WebSocket v2 session.
It owns dial, ping, subscribe, and framed reads for public market channels.
*/
type PublicClient struct {
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
NewPublicClient creates a public websocket client bound to parent context cancellation.
*/
func NewPublicClient(
	ctx context.Context,
	pool *qpool.Q,
	url string,
) *PublicClient {
	ctx, cancel := context.WithCancel(ctx)

	client := &PublicClient{
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

func (publicClient *PublicClient) Start() error {
	return publicClient.Connect()
}

func (publicClient *PublicClient) State() engine.State {
	return engine.READY
}

func (publicClient *PublicClient) Tick() error {
	select {
	case <-publicClient.ctx.Done():
		publicClient.cancel()
		return publicClient.ctx.Err()
	case msg := <-publicClient.subscribers["subscriptions"].Incoming:
		if msg, ok := msg.Value.([]string); !ok {
			return errnie.Error(fmt.Errorf("invalid subscriptions message: %v", msg))
		}

		for _, symbol := range msg.Value.([]string) {
			subscription := errnie.Does(func() (*ohlc.Subscribe, error) {
				return ohlc.NewSubscribe([]string{symbol}), nil
			}).Or(func(err error) {
				errnie.Error(err)
			}).Value()

			publicClient.conn.WriteJSON(subscription)
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
func (publicClient *PublicClient) Connect() error {
	publicClient.conn, _, publicClient.err = websocket.DefaultDialer.DialContext(
		publicClient.ctx, publicClient.url, nil,
	)

	if publicClient.err != nil {
		return errnie.Error(publicClient.err)
	}

	publicClient.ReadLoop()
	return nil
}

/*
Close cancels the session context and closes the underlying socket.
*/
func (publicClient *PublicClient) Close() error {
	publicClient.cancel()
	return publicClient.conn.Close()
}

func (publicClient *PublicClient) ReadLoop() {
	publicClient.once.Do(func() {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-publicClient.ctx.Done():
					return
				case <-ticker.C:
					if err := publicClient.conn.WriteJSON(map[string]string{"method": "ping"}); err != nil {
						errnie.Error(err)
					}
				default:
					_, payload, err := publicClient.conn.ReadMessage()

					if err != nil {
						errnie.Error(err)
						continue
					}

					publicClient.broadcasts["ui"].Send(&qpool.QValue[any]{
						Value: payload,
					})
				}
			}
		}()
	})
}
