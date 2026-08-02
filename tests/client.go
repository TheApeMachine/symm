package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/tests/fixtures/instrument"
	"github.com/theapemachine/symm/tests/fixtures/orderack"
	"github.com/theapemachine/symm/tests/types"
)

var orderCounter atomic.Uint64

/*
Conn is a fake Kraken WebSocket endpoint. It runs a real websocket server on
a local listener, and hands the spot client a Dial function pointing at it,
so the SDK genuinely connects, reads, and writes. REST calls are intercepted
by an in-memory transport. No traffic leaves the machine.
*/
type Conn struct {
	ctx       context.Context
	cancel    context.CancelFunc
	ws        *spot.WebSocket
	transport *mockTransport
	server    *httptest.Server

	mu       sync.Mutex
	accepted *websocket.Conn
	ready    chan struct{}
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
		ready:     make(chan struct{}),
	}

	upgrader := websocket.Upgrader{}

	conn.server = httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			accepted, err := upgrader.Upgrade(w, r, nil)

			if err != nil {
				return
			}

			conn.mu.Lock()
			conn.accepted = accepted
			conn.mu.Unlock()

			select {
			case <-conn.ready:
			default:
				close(conn.ready)
			}

			conn.serve(accepted)
		},
	))

	/*
		The SDK dials this fixture's own listener instead of Kraken, so
		Connect, the read loop, and WriteMessage all run unmodified.
	*/
	client.Dial = func(string) (*websocket.Conn, error) {
		conn.mu.Lock()
		server := conn.server
		conn.mu.Unlock()

		if server == nil {
			return nil, errnie.Err(
				errnie.IO, "tests: fixture websocket is closed", nil,
			)
		}

		dialed, _, err := websocket.DefaultDialer.Dial(
			"ws"+strings.TrimPrefix(server.URL, "http"), nil,
		)

		return dialed, err
	}

	/*
		Connect immediately so the fixture behaves like an already-established
		Kraken session. Live calls Connect again when it wraps this client,
		which simply redials the same listener.
	*/
	errnie.Error(client.Connect())

	select {
	case <-conn.ready:
	case <-time.After(5 * time.Second):
		errnie.Error(errnie.Err(
			errnie.IO, "tests: fixture websocket did not connect", nil,
		))
	}

	return conn
}

/*
serve reads every frame the system under test writes, and answers the
requests that Kraken would acknowledge.
*/
func (conn *Conn) serve(accepted *websocket.Conn) {
	for {
		_, raw, err := accepted.ReadMessage()

		if err != nil {
			return
		}

		conn.handleSentMessage(sdkkraken.NewWebSocketMessage(raw))
	}
}

/*
Configure injects the simulated symbol list into the REST transport so that
Assets, AssetPairs, Balance, and TradeVolume responses are generated from
the fixture system. Must be called before wiring the client into Live.
*/
func (conn *Conn) Configure(symbols []*types.Symbol) {
	conn.transport.configure(symbols)
}

func (conn *Conn) Client() *spot.WebSocket {
	return conn.ws
}

/*
Publish writes a raw JSON payload to the connected client over the fixture's
websocket, so the SDK receives it through its own read loop exactly as it
would a frame from Kraken.
*/
func (conn *Conn) Publish(channel string, payload []byte) {
	if len(payload) == 0 {
		return
	}

	select {
	case <-conn.ready:
	case <-conn.ctx.Done():
		return
	}

	conn.mu.Lock()
	accepted := conn.accepted
	conn.mu.Unlock()

	if accepted == nil {
		return
	}

	/*
		The SDK reads on its own goroutine, so wait until this exact frame has
		been dispatched to OnReceived before returning. That keeps Tick
		deterministic: when it returns, the system has seen every frame.
	*/
	delivered := make(chan struct{})

	handler := conn.ws.OnReceived.Recurring(
		func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
			if bytes.Equal(event.Data.Bytes(), payload) {
				select {
				case <-delivered:
				default:
					close(delivered)
				}
			}
		},
	)

	defer conn.ws.OnReceived.Deregister(handler)

	if err := accepted.WriteMessage(
		websocket.TextMessage, payload,
	); err != nil {
		errnie.Error(err)
		return
	}

	select {
	case <-delivered:
	case <-conn.ctx.Done():
	case <-time.After(5 * time.Second):
		errnie.Error(errnie.Err(
			errnie.IO, "tests: frame delivery timed out", nil,
		))
	}
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
		conn.handleAddOrder()
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

	/*
		Kraken follows a subscription ack with the channel's opening snapshot.
		The instrument channel is the one the boot sequence blocks on, so it
		must be answered for the system to reach READY.
	*/
	if channel == "instrument" {
		pairs := []string{}

		for _, symbol := range conn.transport.getSymbols() {
			pairs = append(pairs, symbol.Pair)
		}

		if len(pairs) == 0 {
			return
		}

		for frame := range instrument.NewMarket(pairs, 0.01).Generate() {
			conn.Publish("instrument", frame)
		}
	}
}

func (conn *Conn) handleAddOrder() {
	sequence := orderCounter.Add(1)

	ackPayload := orderack.Frame(orderack.Options{
		ReqID:   int64(sequence),
		OrderID: fmt.Sprintf("SIM-ORD-%06d", sequence),
		Success: true,
	})

	conn.Publish("add_order", ackPayload)
}

func (conn *Conn) Close() {
	conn.cancel()
	conn.ws.DoReconnect = false

	conn.mu.Lock()
	accepted, server := conn.accepted, conn.server
	conn.accepted, conn.server = nil, nil
	conn.mu.Unlock()

	if accepted != nil {
		errnie.Error(accepted.Close())
	}

	if server != nil {
		server.Close()
	}
}
