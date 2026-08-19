package tests

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/tests/types"
)

const (
	fixtureDeliveryTimeout = 5 * time.Second
	fixtureReconnectWait   = 10 * time.Millisecond
)

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
	faults    *faultInjector
	responder *fixtureResponder
	clock     time.Time

	mu                   sync.Mutex
	writeMu              sync.Mutex
	accepted             *websocket.Conn
	connectionGeneration uint64
	ready                chan struct{}
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
		faults:    newFaultInjector(types.FaultConfig{}),
		clock:     types.DefaultScenarioStart,
		ready:     make(chan struct{}),
	}
	conn.responder = &fixtureResponder{conn: conn}
	client.ReconnectWait = fixtureReconnectWait

	upgrader := websocket.Upgrader{}

	conn.server = httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			accepted, err := upgrader.Upgrade(w, r, nil)

			if err != nil {
				return
			}

			conn.mu.Lock()
			conn.accepted = accepted
			conn.connectionGeneration++
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
	client.URL = "ws" + strings.TrimPrefix(conn.server.URL, "http")

	return conn
}

/*
Connect establishes the fixture websocket for direct SDK tests. Production-like
tests pass Client() to Live, which owns its connection lifecycle instead.
*/
func (conn *Conn) Connect() error {
	if err := conn.ws.Connect(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"tests: fixture websocket failed to connect",
			err,
		))
	}

	select {
	case <-conn.ready:
		return nil
	case <-time.After(fixtureDeliveryTimeout):
		return errnie.Error(errnie.Err(
			errnie.IO,
			"tests: fixture websocket did not connect",
			nil,
		))
	}
}

/*
WaitReady blocks until the fixture websocket has accepted its subscription
connection or the context ends. Replay publishes frames immediately after a
stack boot, while the SDK dials the fixture listener asynchronously; without
this gate the first frames race the handshake and are dropped as undelivered.
*/
func (conn *Conn) WaitReady(ctx context.Context) error {
	for {
		select {
		case <-conn.ready:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(fixtureDeliveryTimeout):
			return errnie.Error(errnie.Err(
				errnie.IO,
				"tests: fixture websocket did not connect",
				nil,
			))
		}
	}
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

		conn.responder.Handle(sdkkraken.NewWebSocketMessage(raw))
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

/*
ConfigureFaults installs deterministic websocket and REST delivery faults.
*/
func (conn *Conn) ConfigureFaults(config types.FaultConfig) {
	conn.mu.Lock()
	conn.faults = newFaultInjector(config)
	conn.mu.Unlock()
	conn.transport.configureFaults(conn.faults)
}

/*
ConfigureAccount injects the wallet and fill history returned during boot.
*/
func (conn *Conn) ConfigureAccount(
	balances map[string]string,
	trades map[string]spot.Trade,
) {
	conn.transport.configureAccount(balances, trades)
}

/*
ConfigureOpenOrders injects working venue orders returned during boot.
*/
func (conn *Conn) ConfigureOpenOrders(orders map[string]spot.Order) {
	conn.transport.configureOpenOrders(orders)
}

/*
FailAddOrder makes the fixture REST transport return err for order submissions.
*/
func (conn *Conn) FailAddOrder(err error) {
	conn.transport.mu.Lock()
	conn.transport.addOrderErr = err
	conn.transport.mu.Unlock()
}

func (conn *Conn) Client() *spot.WebSocket {
	return conn.ws
}

/*
Publish writes a raw JSON payload to the connected client over the fixture's
websocket, so the SDK receives it through its own read loop exactly as it
would a frame from Kraken.
*/
func (conn *Conn) Publish(channel string, payload []byte) bool {
	if len(payload) == 0 {
		return false
	}

	select {
	case <-conn.ready:
	case <-conn.ctx.Done():
		return false
	default:
		return false
	}

	conn.mu.Lock()
	faults := conn.faults
	conn.mu.Unlock()

	if faults == nil {
		return false
	}

	delivery := faults.Apply(channel, payload)

	if delivery.delay > 0 {
		timer := time.NewTimer(delivery.delay)

		select {
		case <-timer.C:
		case <-conn.ctx.Done():
			timer.Stop()
			return false
		}
	}

	if delivery.reconnect {
		conn.reconnect()
		return false
	}

	for _, frame := range delivery.frames {
		conn.publish(frame)
	}

	return true
}

func (conn *Conn) publish(payload []byte) {
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

	// Gorilla permits one writer at a time: the replay pump and subscription
	// responders publish concurrently once the venue stops pacing, so every
	// server-side write serializes here.
	conn.writeMu.Lock()

	if err := accepted.WriteMessage(
		websocket.TextMessage, payload,
	); err != nil {
		conn.writeMu.Unlock()
		errnie.Error(err)
		return
	}

	conn.writeMu.Unlock()

	select {
	case <-delivered:
	case <-conn.ctx.Done():
	case <-time.After(fixtureDeliveryTimeout):
		errnie.Error(errnie.Err(
			errnie.IO, "tests: frame delivery timed out", nil,
		))
	}
}

func (conn *Conn) Close() {
	conn.cancel()

	conn.mu.Lock()
	accepted, server := conn.accepted, conn.server
	conn.accepted, conn.server = nil, nil
	conn.mu.Unlock()

	if accepted != nil {
		errnie.Error(accepted.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second),
		))
		errnie.Error(accepted.Close())
	}

	if server != nil {
		server.Close()
	}
}

func (conn *Conn) Accepted() *websocket.Conn {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	return conn.accepted
}
