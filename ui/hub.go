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
	telemetry     *TelemetryBuffer
	heartbeatSeq  atomic.Uint64

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
		telemetry:     NewTelemetryBuffer(config.System.UITelemetryBuffer),
	}

	hub.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	hub.subscriptions["ui"] = hub.broadcasts["ui"].Subscribe("ui", 128)

	initial := map[string]struct{}{anchorSymbol: {}}
	hub.focus.Store(&initial)

	go hub.writePump(hub.subscriptions["ui"])
	go hub.telemetry.Run(hub.ctx, hub.deliverTelemetry)
	go hub.runHeartbeat()

	return hub, errnie.Require(map[string]any{
		"ctx":           hub.ctx,
		"cancel":        hub.cancel,
		"pool":          hub.pool,
		"broadcasts":    hub.broadcasts,
		"subscriptions": hub.subscriptions,
		"clients":       hub.clients,
		"telemetry":     hub.telemetry,
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
writePump drains qpool into the lossy telemetry ring. Websocket fanout runs
from that ring so the qpool subscriber never waits on browser pressure.
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

			hub.telemetry.Push(value.Value)
		}
	}
}

func (hub *Hub) deliverTelemetry(payload any) {
	hub.maybeUpdateFocus(payload)

	if hub.shouldDrop(payload) {
		runstats.UIFramesFiltered(1)
		return
	}

	hub.fanout(payload)
}

func (hub *Hub) runHeartbeat() {
	interval := config.System.UIHeartbeatInterval

	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lastDropped := int64(0)

	for {
		select {
		case <-hub.ctx.Done():
			return
		case <-ticker.C:
			dropped := hub.telemetry.Dropped()
			droppedDelta := dropped - lastDropped
			lastDropped = dropped

			hub.fanoutPriority(map[string]any{
				"event":         "heartbeat",
				"ts":            time.Now().UTC().Format(time.RFC3339Nano),
				"seq":           hub.heartbeatSeq.Add(1),
				"queue_depth":   hub.telemetry.Depth(),
				"queue_cap":     hub.telemetry.Capacity(),
				"dropped":       dropped,
				"dropped_delta": droppedDelta,
				"throttled":     droppedDelta > 0,
			})
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

func (hub *Hub) fanoutPriority(payload any) {
	hub.clients.Range(func(key, _ any) bool {
		client, ok := key.(*wsClient)

		if !ok {
			return true
		}

		if client.closed.Load() {
			hub.clients.Delete(key)
			return true
		}

		if !client.enqueuePriority(payload) {
			hub.clients.Delete(key)
		}

		return true
	})
}
