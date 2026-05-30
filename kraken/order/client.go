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

const tokenRefreshLead = 30 * time.Second

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
	if err := client.refreshToken(); err != nil {
		return err
	}

	conn, _, err := websocket.DefaultDialer.Dial(string(public.WebSocketAuthURL), nil)

	if err != nil {
		return fmt.Errorf("dial kraken auth websocket: %w", err)
	}

	client.mu.Lock()
	client.conn = conn
	client.mu.Unlock()

	if err := client.writeJSON(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": "executions",
			"token":   client.token,
		},
	}); err != nil {
		return fmt.Errorf("subscribe executions: %w", err)
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

	request.Params.Token = client.token

	if request.ReqID == 0 {
		request.ReqID = int(client.reqID.Add(1))
	}

	return client.writeJSON(request)
}

/*
Fills exposes trade executions parsed from the executions channel.
*/
func (client *Client) Fills() <-chan Fill {
	return client.fills
}

/*
Acks exposes add_order / cancel_order method responses.
*/
func (client *Client) Acks() <-chan Ack {
	return client.acks
}

/*
Close shuts down the trading client.
*/
func (client *Client) Close() error {
	client.cancel()

	client.mu.Lock()
	defer client.mu.Unlock()

	if client.conn != nil {
		_ = client.conn.Close()
		client.conn = nil
	}

	return client.ctx.Err()
}

func (client *Client) refreshToken() error {
	client.mu.Lock()
	defer client.mu.Unlock()

	if client.token != "" && time.Now().Before(client.tokenUntil.Add(-tokenRefreshLead)) {
		return nil
	}

	token, expires, err := client.privateAPI.WebSocketToken(client.ctx)

	if err != nil {
		return err
	}

	client.token = token
	client.tokenUntil = time.Now().Add(expires)

	return nil
}

func (client *Client) writeJSON(payload any) error {
	client.mu.Lock()
	conn := client.conn
	client.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("trading websocket is not connected")
	}

	return conn.WriteJSON(payload)
}

func (client *Client) readLoop() {
	for {
		select {
		case <-client.ctx.Done():
			return
		default:
			client.mu.Lock()
			conn := client.conn
			client.mu.Unlock()

			if conn == nil {
				return
			}

			_, payload, err := conn.ReadMessage()

			if err != nil {
				return
			}

			client.dispatch(payload)
		}
	}
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
