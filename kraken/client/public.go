package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/core"
)

const maxReconnectAttempts = 12

/*
PublicClient maintains an unauthenticated Kraken WebSocket v2 session.
It owns dial, ping, subscribe, and framed reads for public market channels.
*/
type PublicClient struct {
	ctx           context.Context
	cancel        context.CancelFunc
	conn          *websocket.Conn
	wsURL         string
	reqID         int
	handlers      []func(context.Context, []byte) error
	replayFrames  [][]byte
	replayPace    time.Duration
	subscriptions []any
	onDisconnect  func(error)
	onReconnect   func()
	mu            sync.Mutex
	readOnce      sync.Once
	pingOnce      sync.Once
	reconnectMu   sync.Mutex
}

type PublicClientOption func(*PublicClient)

/*
WithWebSocketURL overrides the default Kraken v2 websocket endpoint.
*/
func WithWebSocketURL(url string) PublicClientOption {
	return func(publicClient *PublicClient) {
		publicClient.wsURL = url
	}
}

/*
OnDisconnect registers a callback for unrecoverable read or reconnect failures.
*/
func OnDisconnect(handler func(error)) PublicClientOption {
	return func(publicClient *PublicClient) {
		publicClient.onDisconnect = handler
	}
}

/*
OnReconnect registers a callback after a successful websocket reconnect and resubscribe.
*/
func OnReconnect(handler func()) PublicClientOption {
	return func(publicClient *PublicClient) {
		publicClient.onReconnect = handler
	}
}

/*
NewPublicClient creates a public websocket client bound to parent context cancellation.
*/
func NewPublicClient(parent context.Context, opts ...PublicClientOption) *PublicClient {
	ctx, cancel := context.WithCancel(parent)

	publicClient := &PublicClient{
		ctx:    ctx,
		cancel: cancel,
		wsURL:  core.KRAKEN_WS_URL,
	}

	for _, opt := range opts {
		opt(publicClient)
	}

	return publicClient
}

/*
Connect dials the Kraken v2 websocket endpoint.
Replay sources connect without dialing and require StartReplay after handlers register.
*/
func (publicClient *PublicClient) Connect() error {
	if len(publicClient.replayFrames) > 0 {
		return nil
	}

	if publicClient.conn != nil {
		return fmt.Errorf("public websocket already connected")
	}

	return publicClient.dial()
}

func (publicClient *PublicClient) dial() error {
	conn, _, err := websocket.DefaultDialer.DialContext(publicClient.ctx, publicClient.wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial public websocket: %w", err)
	}

	publicClient.conn = conn
	publicClient.startPing()

	return nil
}

/*
Close cancels the session context and closes the underlying socket.
*/
func (publicClient *PublicClient) Close() error {
	publicClient.cancel()

	publicClient.mu.Lock()
	defer publicClient.mu.Unlock()

	if publicClient.conn == nil {
		return nil
	}

	err := publicClient.conn.Close()
	publicClient.conn = nil

	return err
}

/*
Send marshals and writes a JSON websocket text frame.
Replay mode ignores outbound subscribe frames.
*/
func (publicClient *PublicClient) Send(message any) error {
	if len(publicClient.replayFrames) > 0 {
		return nil
	}

	publicClient.mu.Lock()
	conn := publicClient.conn
	publicClient.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("public websocket is not connected")
	}

	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal websocket message: %w", err)
	}

	return conn.WriteMessage(websocket.TextMessage, payload)
}

/*
Read returns the next websocket text frame payload.
*/
func (publicClient *PublicClient) Read() ([]byte, error) {
	publicClient.mu.Lock()
	conn := publicClient.conn
	publicClient.mu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("public websocket is not connected")
	}

	_, payload, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read public websocket: %w", err)
	}

	return payload, nil
}

/*
Ping sends a Kraken v2 heartbeat request.
*/
func (publicClient *PublicClient) Ping() error {
	return publicClient.Send(map[string]any{"method": "ping"})
}

/*
NextReqID returns the next monotonic subscribe request id.
*/
func (publicClient *PublicClient) NextReqID() int {
	publicClient.mu.Lock()
	defer publicClient.mu.Unlock()

	publicClient.reqID++

	return publicClient.reqID
}

/*
Subscribe sends a subscribe or unsubscribe frame unchanged.
*/
func (publicClient *PublicClient) Subscribe(sub *kraken.Subscription) error {
	if sub == nil {
		return fmt.Errorf("subscription is nil")
	}

	return publicClient.Send(sub)
}

/*
SubscribeTo sends a subscribe frame with a fresh request id.
*/
func (publicClient *PublicClient) SubscribeTo(
	params any,
	opts ...kraken.SubscriptionOption,
) error {
	publicClient.trackSubscription(params)

	return publicClient.sendSubscribe(params, opts...)
}

func (publicClient *PublicClient) sendSubscribe(
	params any,
	opts ...kraken.SubscriptionOption,
) error {
	opts = append(opts, kraken.WithReqID(publicClient.NextReqID()))

	return publicClient.Subscribe(kraken.NewSubscribe(params, opts...))
}

/*
UnsubscribeFrom sends an unsubscribe frame with a fresh request id.
*/
func (publicClient *PublicClient) UnsubscribeFrom(
	params any,
	opts ...kraken.SubscriptionOption,
) error {
	opts = append(opts, kraken.WithReqID(publicClient.NextReqID()))

	return publicClient.Subscribe(kraken.NewUnsubscribe(params, opts...))
}

/*
OnFrame registers a frame handler and starts the shared read loop once.
*/
func (publicClient *PublicClient) OnFrame(handler func(context.Context, []byte) error) {
	publicClient.mu.Lock()
	publicClient.handlers = append(publicClient.handlers, handler)
	publicClient.mu.Unlock()

	publicClient.readOnce.Do(func() {
		go publicClient.dispatchLoop()
	})
}

func (publicClient *PublicClient) dispatchLoop() {
	for {
		if err := publicClient.ctx.Err(); err != nil {
			return
		}

		if publicClient.ReplayMode() {
			return
		}

		payload, err := publicClient.Read()
		if err != nil {
			if publicClient.ctx.Err() != nil {
				return
			}

			if reconnectErr := publicClient.reconnect(); reconnectErr != nil {
				publicClient.notifyDisconnect(reconnectErr)

				return
			}

			continue
		}

		publicClient.mu.Lock()
		handlers := publicClient.handlers
		publicClient.mu.Unlock()

		for _, handler := range handlers {
			if err := handler(publicClient.ctx, payload); err != nil {
				_ = errnie.Error(err)
			}
		}
	}
}

func (publicClient *PublicClient) reconnect() error {
	publicClient.reconnectMu.Lock()
	defer publicClient.reconnectMu.Unlock()

	publicClient.mu.Lock()

	if publicClient.conn != nil {
		_ = publicClient.conn.Close()
		publicClient.conn = nil
	}

	publicClient.mu.Unlock()

	backoff := time.Second

	for attempt := 0; attempt < maxReconnectAttempts; attempt++ {
		if err := publicClient.ctx.Err(); err != nil {
			return err
		}

		if err := publicClient.dial(); err == nil {
			if err := publicClient.resubscribeAll(); err != nil {
				return err
			}

			errnie.Warn("public websocket reconnected", "attempt", attempt+1)

			if publicClient.onReconnect != nil {
				publicClient.onReconnect()
			}

			return nil
		}

		timer := time.NewTimer(backoff)

		select {
		case <-publicClient.ctx.Done():
			timer.Stop()

			return publicClient.ctx.Err()
		case <-timer.C:
		}

		if backoff < 30*time.Second {
			backoff *= 2
		}
	}

	return fmt.Errorf("public websocket reconnect failed after %d attempts", maxReconnectAttempts)
}

func (publicClient *PublicClient) trackSubscription(params any) {
	if params == nil {
		return
	}

	publicClient.mu.Lock()
	publicClient.subscriptions = append(publicClient.subscriptions, params)
	publicClient.mu.Unlock()
}

func (publicClient *PublicClient) resubscribeAll() error {
	publicClient.mu.Lock()
	paramsList := append([]any(nil), publicClient.subscriptions...)
	publicClient.mu.Unlock()

	for index, params := range paramsList {
		if err := publicClient.sendSubscribe(params); err != nil {
			return fmt.Errorf("resubscribe frame %d: %w", index, err)
		}
	}

	return nil
}

func (publicClient *PublicClient) startPing() {
	if publicClient.ReplayMode() {
		return
	}

	publicClient.pingOnce.Do(func() {
		go func() {
			interval := config.System.WSPingInterval

			if interval <= 0 {
				interval = 30 * time.Second
			}

			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				select {
				case <-publicClient.ctx.Done():
					return
				case <-ticker.C:
					if err := publicClient.Ping(); err != nil {
						errnie.Warn("public websocket ping failed", "err", err)
					}
				}
			}
		}()
	})
}

func (publicClient *PublicClient) notifyDisconnect(err error) {
	if err == nil {
		return
	}

	_ = errnie.Error(err)

	if publicClient.onDisconnect != nil {
		publicClient.onDisconnect(err)
	}
}

/*
StartReader runs a blocking read loop until context cancellation or handler error.
*/
func (publicClient *PublicClient) StartReader(
	handler func(context.Context, []byte) error,
) error {
	if publicClient.conn == nil {
		return fmt.Errorf("public websocket is not connected")
	}

	for {
		if err := publicClient.ctx.Err(); err != nil {
			return nil
		}

		payload, err := publicClient.Read()
		if err != nil {
			if publicClient.ctx.Err() != nil {
				return nil
			}

			return err
		}

		if err := handler(publicClient.ctx, payload); err != nil {
			return err
		}
	}
}
