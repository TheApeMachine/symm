package order

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/theapemachine/symm/kraken/private"
	"github.com/theapemachine/symm/kraken/public"
)

const (
	tokenRefreshLead  = 30 * time.Second
	reconnectBackoff  = 2 * time.Second
	maxReconnectPause = 30 * time.Second
)

/*
Client is the authenticated Kraken WebSocket v2 trading connection.
*/
type Client struct {
	ctx        context.Context
	cancel     context.CancelFunc
	privateAPI *private.Rest
	conn       *websocket.Conn
	token      string
	tokenUntil time.Time
	reqID      atomic.Uint64
	fills      chan Fill
	acks       chan Ack
	mu         sync.Mutex
	writeMu    sync.Mutex
}

/*
NewClient wires private REST credentials for token refresh and order routing.
*/
func NewClient(ctx context.Context, apiKey, apiSecret string) (*Client, error) {
	privateAPI, err := private.NewRest(apiKey, apiSecret)

	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)

	return &Client{
		ctx:        ctx,
		cancel:     cancel,
		privateAPI: privateAPI,
		fills:      make(chan Fill, 128),
		acks:       make(chan Ack, 64),
	}, nil
}

/*
Start dials the auth socket, subscribes to executions, and begins reading frames.
*/
func (client *Client) Start() error {
	if err := client.connect(); err != nil {
		return err
	}

	go client.readLoop()

	return nil
}

/*
Publish sends one trading request frame with a fresh token when needed.
*/
func (client *Client) Publish(request Request) error {
	if err := client.refreshToken(); err != nil {
		return err
	}

	client.mu.Lock()
	token := client.token
	client.mu.Unlock()

	request.Params.Token = token

	if request.ReqID == 0 {
		request.ReqID = int(client.reqID.Add(1))
	}

	return client.writeJSON(request)
}

/*
PublishCancel sends one cancel_order frame with a fresh token when needed.
*/
func (client *Client) PublishCancel(request CancelRequest) error {
	if err := client.refreshToken(); err != nil {
		return err
	}

	client.mu.Lock()
	token := client.token
	client.mu.Unlock()

	request.Params.Token = token

	if request.ReqID == 0 {
		request.ReqID = int(client.reqID.Add(1))
	}

	return client.writeJSON(request)
}

func (client *Client) Fills() <-chan Fill {
	return client.fills
}

func (client *Client) Acks() <-chan Ack {
	return client.acks
}

/*
Close shuts down the trading client and returns any connection close error.
*/
func (client *Client) Close() error {
	client.cancel()

	client.mu.Lock()
	defer client.mu.Unlock()

	if client.conn == nil {
		return nil
	}

	closeErr := client.conn.Close()
	client.conn = nil

	return closeErr
}

func (client *Client) refreshToken() error {
	client.mu.Lock()

	if client.token != "" && time.Now().Before(client.tokenUntil.Add(-tokenRefreshLead)) {
		client.mu.Unlock()

		return nil
	}

	client.mu.Unlock()

	token, expires, err := client.privateAPI.WebSocketToken(client.ctx)

	if err != nil {
		return err
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	if client.token != "" && time.Now().Before(client.tokenUntil.Add(-tokenRefreshLead)) {
		return nil
	}

	client.token = token
	client.tokenUntil = time.Now().Add(expires)

	return nil
}

func (client *Client) connect() error {
	if err := client.refreshToken(); err != nil {
		return err
	}

	conn, _, err := websocket.DefaultDialer.Dial(string(public.WebSocketAuthURL), nil)

	if err != nil {
		return fmt.Errorf("dial kraken auth websocket: %w", err)
	}

	client.mu.Lock()
	client.conn = conn
	token := client.token
	client.mu.Unlock()

	if err := client.writeJSON(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": "executions",
			"token":   token,
		},
	}); err != nil {
		client.closeConn()

		return fmt.Errorf("subscribe executions: %w", err)
	}

	return nil
}

func (client *Client) closeConn() {
	client.mu.Lock()
	defer client.mu.Unlock()

	if client.conn == nil {
		return
	}

	_ = client.conn.Close()
	client.conn = nil
}

func (client *Client) writeJSON(payload any) error {
	client.mu.Lock()
	conn := client.conn
	client.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("trading websocket is not connected")
	}

	client.writeMu.Lock()
	defer client.writeMu.Unlock()

	return conn.WriteJSON(payload)
}

func (client *Client) readLoop() {
	backoff := reconnectBackoff

	for {
		if client.ctx.Err() != nil {
			return
		}

		client.mu.Lock()
		conn := client.conn
		client.mu.Unlock()

		if conn == nil {
			if err := client.reconnectWithBackoff(&backoff); err != nil {
				return
			}

			continue
		}

		_, payload, err := conn.ReadMessage()

		if err != nil {
			client.closeConn()

			if client.ctx.Err() != nil {
				return
			}

			if reconnectErr := client.reconnectWithBackoff(&backoff); reconnectErr != nil {
				return
			}

			continue
		}

		backoff = reconnectBackoff
		client.dispatch(payload)
	}
}

func (client *Client) reconnectWithBackoff(backoff *time.Duration) error {
	for client.ctx.Err() == nil {
		if err := client.connect(); err != nil {
			select {
			case <-client.ctx.Done():
				return client.ctx.Err()
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

	return client.ctx.Err()
}

func (client *Client) dispatch(payload []byte) {
	var envelope struct {
		Channel string `json:"channel"`
		Method  string `json:"method"`
	}

	if err := sonic.Unmarshal(payload, &envelope); err != nil {
		return
	}

	if envelope.Channel == "executions" {
		fills, err := ParseExecutionFills(payload)

		if err != nil {
			return
		}

		client.enqueueFills(fills)

		return
	}

	if envelope.Method == "" {
		return
	}

	ack, err := ParseAck(payload)

	if err != nil {
		return
	}

	client.enqueueAck(*ack)
}

func (client *Client) enqueueFills(fills []Fill) {
	for _, fill := range fills {
		if !client.enqueueFill(fill) {
			return
		}
	}
}

func (client *Client) enqueueFill(fill Fill) bool {
	if client.ctx == nil {
		client.fills <- fill

		return true
	}

	select {
	case client.fills <- fill:
		return true
	case <-client.ctx.Done():
		return false
	}
}

func (client *Client) enqueueAck(ack Ack) {
	if client.ctx == nil {
		client.acks <- ack

		return
	}

	select {
	case client.acks <- ack:
	case <-client.ctx.Done():
	}
}
