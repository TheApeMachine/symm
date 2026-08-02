package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/tests/fixtures/orderack"
)

var orderCounter uint64

/*
Conn wraps a spot.WebSocket client with an in-memory transport layer so that
all REST calls and WebSocket write/subscribe operations are intercepted and
handled by the fixture system without any network traffic.
*/
type Conn struct {
	ctx       context.Context
	cancel    context.CancelFunc
	ws        *spot.WebSocket
	transport *mockTransport
}

/*
NewConn constructs a Conn backed by in-memory fixture transport.
Call Configure() with the symbol list before wiring the client into a
Live instance, so that REST endpoints return proper asset/pair/fee data.
*/
func NewConn(ctxs ...context.Context) *Conn {
	ctx := context.Background()

	if len(ctxs) > 0 && ctxs[0] != nil {
		ctx = ctxs[0]
	}

	ctx, cancel := context.WithCancel(ctx)

	client := spot.NewWebSocket()
	transport := newMockTransport()

	client.REST.Executor = transport.RoundTrip

	conn := &Conn{
		ctx:       ctx,
		cancel:    cancel,
		ws:        client,
		transport: transport,
	}

	conn.ws.OnSent.Recurring(
		func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
			conn.handleSentMessage(event.Data)
		},
	)

	return conn
}

/*
Configure injects the simulated symbol list into the REST transport so that
Assets, AssetPairs, Balance, and TradeVolume responses are generated from
the fixture system. Must be called before wiring the client into Live.
*/
func (conn *Conn) Configure(symbols []*Symbol) {
	conn.transport.configure(symbols)
}

func (conn *Conn) Client() *spot.WebSocket {
	return conn.ws
}

/*
Publish pushes a raw JSON payload directly into the client's OnReceived
callback chain, exactly as if it arrived over a real WebSocket connection.
*/
func (conn *Conn) Publish(channel string, payload []byte) {
	if len(payload) == 0 {
		return
	}

	conn.ws.OnReceived.Call(sdkkraken.NewWebSocketMessage(payload))
}

func (conn *Conn) handleSentMessage(msg *sdkkraken.WebSocketMessage) {
	if msg == nil {
		return
	}

	raw := msg.Bytes()

	if len(raw) == 0 {
		return
	}

	var wire map[string]any

	if err := json.Unmarshal(raw, &wire); err != nil {
		return
	}

	method, _ := wire["method"].(string)

	if method == "" {
		return
	}

	switch method {
	case "subscribe":
		conn.handleSubscribe(wire)
	case "add_order":
		conn.handleAddOrder(wire)
	}
}

func (conn *Conn) handleSubscribe(wire map[string]any) {
	params, _ := wire["params"].(map[string]any)
	channel, _ := params["channel"].(string)
	reqID := wire["req_id"]

	now := time.Now().UTC().Format(time.RFC3339Nano)

	ack, _ := json.Marshal(map[string]any{
		"method":  "subscribe",
		"req_id":  reqID,
		"success": true,
		"result": map[string]any{
			"channel": channel,
		},
		"time_in":  now,
		"time_out": now,
	})

	conn.Publish("subscribe", ack)
}

func (conn *Conn) handleAddOrder(wire map[string]any) {
	sequence := atomic.AddUint64(&orderCounter, 1)

	ackPayload := orderack.Frame(orderack.Options{
		ReqID:   int64(sequence),
		OrderID: fmt.Sprintf("SIM-ORD-%06d", sequence),
		Success: true,
	})

	conn.Publish("add_order", ackPayload)
}

func (conn *Conn) Close() {
	conn.cancel()
}
