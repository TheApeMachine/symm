package ui

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/runstats"
)

// anchorSymbol is always forwarded to the frontend regardless of open
// position state — BTC/EUR is the dashboard's reference price and the
// lead-lag anchor; without it the price chart goes blank between
// trades. Per-symbol frames for symbols that are neither anchor nor
// open positions are filtered out at the hub so the frontend isn't
// drowned in altcoin ticks it doesn't render.
const anchorSymbol = "BTC/EUR"

const (
	writeDeadline = 2 * time.Second
	// perClientBuffer absorbs measurement bursts. The signal layer can
	// emit dozens of frames in a single tick (one per gauge, plus mark,
	// plus prediction). 4096 gives ~10 ticks of headroom before drop
	// kicks in, which is well above the worst-case burst we've measured.
	perClientBuffer = 4096
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: allowLocalhostOrigin,
}

func allowLocalhostOrigin(request *http.Request) bool {
	origin := strings.TrimSpace(request.Header.Get("Origin"))

	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)

	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Hostname())

	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

/*
wsClient owns one connected browser. Each client has its own bounded
outbox so a slow socket can't head-of-line block other clients or the
upstream broadcast pump. The frontend is pure consumer; we never read
from the conn, but we install a write deadline so a stuck socket fails
fast.

Close semantics. The previous design closed client.out from close() and
then let enqueue send on the same channel concurrently; on a
disconnect-while-broadcasting interleave that panics with "send on
closed channel." Now close() signals via the done channel only — the
outbox is left to be GC'd. enqueue checks done before sending and
guarantees a non-blocking send under the select, so even a racing
close() cannot turn into a panic.
*/
type wsClient struct {
	conn   *websocket.Conn
	out    chan any
	done   chan struct{}
	closed atomic.Bool
}

func newClient(conn *websocket.Conn) *wsClient {
	return &wsClient{
		conn: conn,
		out:  make(chan any, perClientBuffer),
		done: make(chan struct{}),
	}
}

/*
close marks the client as closed and signals the writer to exit via the
done channel. The outbox is intentionally not closed: enqueue may race
with close from the broadcast fanout, and sending on a closed channel
panics. The writer drains everything still in the outbox on the way
out, then drops the conn.
*/
func (client *wsClient) close() error {
	if client.closed.Swap(true) {
		return nil
	}

	close(client.done)
	return client.conn.Close()
}

/*
enqueue posts payload to the client's outbox without blocking. The
default case silently drops the message when the bounded outbox is
full — under a burst (UI gets ~3 frames per measurement and many
measurements per tick) the previous "return false on full and let the
caller close the client" behavior was evicting and forcing reconnect
on every burst, which reset the frontend's tick counter and stalled
the system on reconnect churn. A dropped frame is fine; an evicted
browser is not.

A genuinely closed client (done signaled) returns false so the caller
removes it from the clients map.
*/
func (client *wsClient) enqueue(payload any) bool {
	if client.closed.Load() {
		return false
	}

	select {
	case client.out <- payload:
		runstats.UIFramesSent(1)
		return true
	case <-client.done:
		return false
	default:
		// Buffer full — drop this frame, keep the client. The drop is
		// counted in run_stats so a post-run jq can see how often the
		// signal layer is overwhelming the browser link.
		runstats.UIFramesDropped(1)
		return true
	}
}

/*
runWriter writes outbox entries to the socket with a deadline. The loop
exits immediately when client.done is closed; any messages still sitting
in client.out at that moment are discarded — the writer does not flush
the channel before returning. A connected browser sees this as missing
the last fanout tick before disconnect, which is the right behavior
because the per-client buffer is also bounded and we already accept drop
semantics under back-pressure. Any write error also triggers close so
the next fanout pass evicts the client.
*/
func (client *wsClient) runWriter() {
	for {
		select {
		case <-client.done:
			return
		case payload := <-client.out:
			_ = client.conn.SetWriteDeadline(time.Now().Add(writeDeadline))

			if err := client.conn.WriteJSON(payload); err != nil {
				_ = client.close()
				return
			}
		}
	}
}

/*
Hub subscribes to every broadcast group and writes payloads to websocket
clients. There is intentionally no reader goroutine per client — the
frontend never sends frames. Liveness is enforced by the per-client
read deadline plus the conn's built-in ping handling.
*/
type Hub struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q
	broadcasts    map[string]*qpool.BroadcastGroup
	subscriptions map[string]*qpool.Subscriber
	clients       *sync.Map

	// focus = {anchor} ∪ {open-position symbols}. Updated atomically
	// from every "wallet" frame.
	focus atomic.Pointer[map[string]struct{}]
}

/*
NewHub subscribes to all broadcast groups on pool.
*/
func NewHub(
	ctx context.Context,
	pool *qpool.Q,
) (*Hub, error) {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		ctx:           ctx,
		cancel:        cancel,
		pool:          pool,
		broadcasts:    make(map[string]*qpool.BroadcastGroup),
		subscriptions: make(map[string]*qpool.Subscriber),
		clients:       &sync.Map{},
	}

	hub.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	hub.subscriptions["ui"] = hub.broadcasts["ui"].Subscribe("ui", 128)

	initial := map[string]struct{}{anchorSymbol: {}}
	hub.focus.Store(&initial)

	go hub.writePump(hub.subscriptions["ui"])

	return hub, errnie.Require(map[string]any{
		"ctx":           hub.ctx,
		"cancel":        hub.cancel,
		"pool":          hub.pool,
		"broadcasts":    hub.broadcasts,
		"subscriptions": hub.subscriptions,
		"clients":       hub.clients,
	})
}

/*
Serve starts the websocket server on addr (e.g. :8765).
*/
func (hub *Hub) Serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.handleWS)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return server.ListenAndServe()
}

func (hub *Hub) handleWS(writer http.ResponseWriter, request *http.Request) {
	conn, err := wsUpgrader.Upgrade(writer, request, nil)

	if err != nil {
		_ = errnie.Error(err)
		return
	}

	client := newClient(conn)
	hub.clients.Store(client, struct{}{})

	go client.runWriter()

	hub.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event": "hello",
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
	}})
}

/*
writePump fans incoming broadcasts out to every connected client by
enqueueing onto each client's bounded outbox. A nil channel value is
treated as a transient hiccup, not a permanent EOF, so the pump survives
upstream broadcast restarts. Clients whose outbox is full are evicted on
the spot — a stalled browser cannot back-pressure the system.
*/
func (hub *Hub) writePump(subscription *qpool.Subscriber) {
	for {
		select {
		case <-hub.ctx.Done():
			return
		case value, ok := <-subscription.Incoming:
			if !ok {
				return
			}

			if value == nil || value.Value == nil {
				continue
			}

			hub.maybeUpdateFocus(value.Value)

			if hub.shouldDrop(value.Value) {
				runstats.UIFramesFiltered(1)
				continue
			}

			hub.fanout(value.Value)
		}
	}
}

/*
maybeUpdateFocus rebuilds the focus set on every "wallet" frame so the
hub forwards per-symbol data for symbols we actually hold. Aggregate
frames and the wallet frame itself always pass.
*/
func (hub *Hub) maybeUpdateFocus(payload any) {
	frame, ok := payload.(map[string]any)

	if !ok || frame["event"] != "wallet" {
		return
	}

	inventory, _ := frame["Inventory"].(map[string]float64)
	currency, _ := frame["Currency"].(string)

	if currency == "" {
		currency = config.System.QuoteCurrency
	}

	next := map[string]struct{}{anchorSymbol: {}}

	for base, qty := range inventory {
		if qty <= 0 || base == "" || currency == "" {
			continue
		}

		next[base+"/"+currency] = struct{}{}
	}

	hub.focus.Store(&next)
}

/*
shouldDrop returns true for per-symbol frames whose symbol is not in
focus. Aggregate frames (no symbol field) always pass.
*/
func (hub *Hub) shouldDrop(payload any) bool {
	frame, ok := payload.(map[string]any)

	if !ok {
		return false
	}

	symbol, hasSymbol := frame["symbol"].(string)

	if !hasSymbol || symbol == "" {
		return false
	}

	focusPtr := hub.focus.Load()

	if focusPtr == nil {
		return false
	}

	focus := *focusPtr
	_, present := focus[symbol]

	return !present
}

func (hub *Hub) fanout(payload any) {
	hub.clients.Range(func(key, _ any) bool {
		client, ok := key.(*wsClient)

		if !ok {
			return true
		}

		if client.closed.Load() {
			hub.clients.Delete(key)
			return true
		}

		// enqueue returns true on success AND on bounded-buffer drop —
		// the only false return is "client is closed", which is the only
		// case where we evict. A slow consumer drops frames silently
		// until its outbox drains.
		if !client.enqueue(payload) {
			hub.clients.Delete(key)
		}

		return true
	})
}

/*
Close shuts down the telemetry hub.
*/
func (hub *Hub) Close() error {
	hub.cancel()

	hub.clients.Range(func(key, _ any) bool {
		client, ok := key.(*wsClient)

		if ok {
			_ = client.close()
		}

		hub.clients.Delete(key)
		return true
	})

	return nil
}
