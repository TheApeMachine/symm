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
)

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
Hub subscribes to the "ui" broadcast group and ships whatever lands there to the
websocket clients. Producers decide what to publish and gate per-symbol frames by
open position at the source, so the hub does no filtering — it only buffers
(lossy telemetry ring) and fans out. There is intentionally no reader goroutine
per client; the frontend never sends frames.
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

	// audits is a small ring of recent audit frames replayed to each newly
	// connected client, so the decision log is not empty just because the trades
	// happened before the dashboard was open.
	auditMu sync.Mutex
	audits  []any
}

// auditHistory bounds the replayed audit ring.
const auditHistory = 50

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

	now := time.Now().UTC()
	client.enqueue(map[string]any{
		"event": "hello",
		"ts":    now.Format(time.RFC3339Nano),
	})
	client.enqueue(DefaultDashboardLayout(now).Wire())

	hub.replayAudits(client)
}

// replayAudits sends the recent audit log to one freshly connected client so its
// decision panel is populated with what already happened.
func (hub *Hub) replayAudits(client *wsClient) {
	hub.auditMu.Lock()
	recent := append([]any(nil), hub.audits...)
	hub.auditMu.Unlock()

	for _, frame := range recent {
		client.enqueue(frame)
	}
}

// cacheAudit keeps a bounded ring of audit frames for replay to new clients.
func (hub *Hub) cacheAudit(payload any) {
	frame, ok := payload.(map[string]any)

	if !ok || frame["event"] != "audit" {
		return
	}

	hub.auditMu.Lock()
	defer hub.auditMu.Unlock()

	hub.audits = append(hub.audits, payload)

	if len(hub.audits) > auditHistory {
		hub.audits = hub.audits[len(hub.audits)-auditHistory:]
	}
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
	hub.cacheAudit(payload)
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
