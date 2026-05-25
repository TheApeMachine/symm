package ui

import (
	"context"
	"fmt"
	"maps"
	"net"
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
	"github.com/theapemachine/symm/engine"
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

	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

type wsClient struct {
	conn   *websocket.Conn
	closed atomic.Bool
}

/*
CommandHandler receives websocket control payloads from connected clients.
*/
type CommandHandler interface {
	HandleCommand(raw any)
}

/*
Hub drains the shared ui broadcast group and writes frames to websocket clients.
Producers Send on the group; writePump delivers asynchronously so Kraken websocket
frames are not blocked when the orchestrator is in Measure.
*/
type Hub struct {
	ctx            context.Context
	cancel         context.CancelFunc
	clients        sync.Map
	snapshotMu     sync.RWMutex
	snapshot       map[string]map[string]any
	ui             *qpool.BroadcastGroup
	uiSubscription *qpool.Subscriber
	commands       CommandHandler
	runID          string
}

var dashboardSnapshotOrder = []string{
	"engine_pulse",
	"decision_trace",
	"scoreboard",
	"status",
}

var _ engine.System = (*Hub)(nil)

/*
Start serves the websocket hub when UIAddr is configured.
*/
func (hub *Hub) Start() error {
	addr, ok := ListenAddr(config.System.UIAddr)

	if !ok {
		return nil
	}

	go func() {
		if err := hub.Serve(addr); err != nil {
			errnie.Error(err)
		}
	}()

	return nil
}

func (hub *Hub) State() engine.State {
	return engine.READY
}

func (hub *Hub) Tick() error {
	select {
	case <-hub.ctx.Done():
		return hub.ctx.Err()
	default:
		return nil
	}
}

/*
NewHub subscribes to the ui broadcast group on pool.
*/
func NewHub(
	ctx context.Context,
	pool *qpool.Q,
	commands CommandHandler,
) (*Hub, error) {
	if pool == nil {
		return nil, fmt.Errorf("ui hub requires pool")
	}

	ui := pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		ctx:      ctx,
		cancel:   cancel,
		snapshot: make(map[string]map[string]any, len(dashboardSnapshotOrder)),
		ui:       ui,
		commands: commands,
		runID:    time.Now().UTC().Format("20060102T150405Z"),
	}

	hub.uiSubscription = ui.Subscribe("ui", 65536)

	go hub.writePump()

	return hub, errnie.Require(map[string]any{
		"ctx":            hub.ctx,
		"cancel":         hub.cancel,
		"ui":             ui,
		"uiSubscription": hub.uiSubscription,
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

	go func() {
		<-hub.ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			errnie.Error(err)
		}
	}()

	return server.ListenAndServe()
}

func (hub *Hub) handleWS(writer http.ResponseWriter, request *http.Request) {
	conn, err := wsUpgrader.Upgrade(writer, request, nil)

	if err != nil {
		_ = errnie.Error(err)
		return
	}

	client := &wsClient{
		conn: conn,
	}

	_ = client.conn.WriteJSON(map[string]any{
		"event":  "hello",
		"ts":     time.Now().UTC().Format(time.RFC3339Nano),
		"run_id": hub.runID,
	})

	hub.writeSnapshot(client)
	hub.clients.Store(client, struct{}{})
	hub.readPump(client)
}

func (hub *Hub) writePump() {
	for {
		select {
		case <-hub.ctx.Done():
			return
		case value, open := <-hub.uiSubscription.Incoming:
			if !open {
				return
			}

			if value == nil {
				continue
			}

			hub.broadcastToClients(value)
		}
	}
}

func (hub *Hub) broadcastToClients(value *qpool.QValue[any]) {
	if payload, ok := value.Value.(map[string]any); ok {
		hub.cacheSnapshot(payload)
	}

	if payloads, ok := value.Value.([]map[string]any); ok {
		for _, payload := range payloads {
			hub.cacheSnapshot(payload)
		}
	}

	hub.clients.Range(func(key, _ any) bool {
		client, ok := key.(*wsClient)

		if !ok || client.closed.Load() {
			return true
		}

		switch typed := value.Value.(type) {
		case []byte:
			_ = client.conn.WriteMessage(websocket.TextMessage, typed)
		default:
			_ = client.conn.WriteJSON(value.Value)
		}

		return true
	})
}

func (hub *Hub) writeSnapshot(client *wsClient) {
	for _, payload := range hub.dashboardSnapshot() {
		_ = client.conn.WriteJSON(payload)
	}
}

func (hub *Hub) cacheSnapshot(payload map[string]any) {
	event, _ := payload["event"].(string)

	if !isDashboardSnapshotEvent(event) {
		return
	}

	hub.snapshotMu.Lock()
	defer hub.snapshotMu.Unlock()

	if hub.snapshot == nil {
		hub.snapshot = make(map[string]map[string]any, len(dashboardSnapshotOrder))
	}

	hub.snapshot[event] = maps.Clone(payload)
}

func (hub *Hub) dashboardSnapshot() []map[string]any {
	hub.snapshotMu.RLock()
	defer hub.snapshotMu.RUnlock()

	frames := make([]map[string]any, 0, len(dashboardSnapshotOrder))

	for _, event := range dashboardSnapshotOrder {
		payload, ok := hub.snapshot[event]

		if !ok {
			continue
		}

		frames = append(frames, maps.Clone(payload))
	}

	return frames
}

func isDashboardSnapshotEvent(event string) bool {
	for _, candidate := range dashboardSnapshotOrder {
		if event == candidate {
			return true
		}
	}

	return false
}

func (hub *Hub) readPump(client *wsClient) {
	defer func() {
		client.closed.Store(true)
		hub.clients.Delete(client)
		_ = client.conn.Close()
	}()

	for {
		if hub.ctx.Err() != nil {
			return
		}

		_, payload, err := client.conn.ReadMessage()

		if err != nil {
			return
		}

		if hub.commands != nil {
			hub.commands.HandleCommand(payload)
		}
	}
}

/*
Close shuts down the telemetry hub.
*/
func (hub *Hub) Close() error {
	hub.cancel()

	return errnie.Require(map[string]any{
		"event": "ui_hub_closed",
	})
}

/*
ListenAddr parses config.System.UIAddr into a host listen address (e.g. :8765).
*/
func ListenAddr(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	if strings.HasPrefix(raw, ":") {
		return net.JoinHostPort("127.0.0.1", strings.TrimPrefix(raw, ":")), true
	}

	parsed, err := url.Parse(raw)

	if err != nil {
		return "", false
	}

	if parsed.Host != "" {
		host := parsed.Host

		if strings.Contains(host, ":") {
			return host, true
		}

		return net.JoinHostPort(host, "8765"), true
	}

	return "", false
}
