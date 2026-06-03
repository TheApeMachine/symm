package ui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/market/perspectives"
)

const (
	uiHubSubscriberID       = "ui/hub:ui"
	uiHubChartsSubscriberID = "ui/hub:charts"
	uiResyncChannel         = "ui:resync"
	uiChartsChannel         = "ui:charts"
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
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	clients     *sync.Map
	server      *http.Server
	nextConnID  uint64
	tickSeq     atomic.Int64
}

/*
NewHub subscribes to all broadcast groups on pool.
*/
func NewHub(
	ctx context.Context,
	pool *qpool.Q,
) *Hub {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		clients:     &sync.Map{},
	}

	hub.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	hub.subscribers["ui"] = hub.broadcasts["ui"].Subscribe(uiHubSubscriberID, 1024)
	hub.broadcasts[uiChartsChannel] = pool.CreateBroadcastGroup(uiChartsChannel, 10*time.Millisecond)
	hub.subscribers[uiChartsChannel] = hub.broadcasts[uiChartsChannel].Subscribe(
		uiHubChartsSubscriberID, 4096,
	)
	hub.broadcasts[uiResyncChannel] = pool.CreateBroadcastGroup(uiResyncChannel, 10*time.Millisecond)

	addr := strings.TrimSpace(viper.GetString("ui.addr"))

	if addr == "" {
		cancel()
		errnie.Error(fmt.Errorf("ui.addr must be set"), "ui hub")
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.handleWS)

	listener, err := net.Listen("tcp", addr)

	if err != nil {
		cancel()
		errnie.Error(fmt.Errorf("ui hub listen %q: %w", addr, err), "ui hub")
		return nil
	}

	hub.server = &http.Server{
		Handler: mux,
	}

	activate.Boot("ui/hub listening on " + addr)

	perspectives.DefaultTelemetryRegistry().Subscribe(func() {
		hub.fanoutLayout()
	})

	go func() {
		serveErr := hub.server.Serve(listener)

		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			fmt.Printf("ui hub serve: %v\n", serveErr)
		}
	}()

	return hub
}

func (hub *Hub) Close() error {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if hub.server != nil {
		if err := hub.server.Shutdown(shutdownCtx); err != nil {
			fmt.Printf("ui hub shutdown: %v\n", err)
		}
	}

	hub.clients.Range(func(key, value any) bool {
		conn, ok := value.(*websocket.Conn)

		if ok {
			_ = conn.Close()
		}

		hub.clients.Delete(key)

		return true
	})

	hub.cancel()

	return nil
}

func (hub *Hub) handleWS(writer http.ResponseWriter, request *http.Request) {
	conn, err := wsUpgrader.Upgrade(writer, request, nil)

	if err != nil {
		return
	}

	hello := map[string]any{
		"event": "hello",
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
	}

	if err := conn.WriteJSON(hello); err != nil {
		hub.dropClient(conn, err)
		return
	}

	if err := conn.WriteJSON(LayoutDocument()); err != nil {
		hub.dropClient(conn, err)
		return
	}

	if err := hub.writeHeartbeat(conn, hub.tickSeq.Load()); err != nil {
		hub.dropClient(conn, err)
		return
	}

	connID := atomic.AddUint64(&hub.nextConnID, 1)
	hub.clients.Store(connID, conn)

	if resync := hub.broadcasts[uiResyncChannel]; resync != nil {
		resync.Send(&qpool.QValue[any]{Value: map[string]any{
			"event": "resync",
			"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		}})
	}

	activate.Once("ui/hub:websocket-client")
}

/*
Tick drains qpool into the lossy telemetry ring and fanout to websocket clients.
from that ring so the qpool subscriber never waits on browser pressure.
*/
func (hub *Hub) Tick() error {
	heartbeat := time.NewTicker(hub.heartbeatInterval())

	defer heartbeat.Stop()

	for {
		select {
		case <-hub.ctx.Done():
			return hub.ctx.Err()
		case <-heartbeat.C:
			hub.publishHeartbeat(hub.tickSeq.Add(1))
		case value, ok := <-hub.subscribers["ui"].Incoming:
			if !ok {
				return hub.ctx.Err()
			}

			hub.fanout(value)
		case value, ok := <-hub.subscribers[uiChartsChannel].Incoming:
			if !ok {
				return hub.ctx.Err()
			}

			hub.fanout(value)
		}
	}
}

func (hub *Hub) fanoutLayout() {
	hub.fanoutMap(LayoutDocument())
}

func (hub *Hub) fanout(value *qpool.QValue[any]) {
	if value == nil || value.Value == nil {
		return
	}

	out, ok := value.Value.(map[string]any)

	if !ok {
		return
	}

	hub.fanoutMap(out)
}

func (hub *Hub) fanoutMap(out map[string]any) {
	if out == nil {
		return
	}

	hub.clients.Range(func(key, value any) bool {
		conn, ok := value.(*websocket.Conn)

		if !ok {
			hub.clients.Delete(key)

			return true
		}

		if err := conn.WriteJSON(out); err != nil {
			hub.dropClient(conn, err)
			hub.clients.Delete(key)
		}

		return true
	})
}

func (hub *Hub) heartbeatInterval() time.Duration {
	const defaultHeartbeat = time.Second

	interval := viper.GetDuration("ui.heartbeat_interval")

	if interval <= 0 {
		return defaultHeartbeat
	}

	return interval
}

func (hub *Hub) publishHeartbeat(seq int64) {
	if hub.broadcasts["ui"] == nil {
		return
	}

	hub.broadcasts["ui"].Send(&qpool.QValue[any]{Value: hub.heartbeatFrame(seq)})
}

func (hub *Hub) writeHeartbeat(conn *websocket.Conn, seq int64) error {
	return conn.WriteJSON(hub.heartbeatFrame(seq))
}

func (hub *Hub) heartbeatFrame(seq int64) map[string]any {
	return map[string]any{
		"event": "heartbeat",
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"seq":   seq,
	}
}

func (hub *Hub) dropClient(conn *websocket.Conn, writeErr error) {
	if writeErr != nil && !clientDisconnected(writeErr) {
		fmt.Printf("ui hub client: %v\n", writeErr)
	}

	_ = conn.Close()
}

func clientDisconnected(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, net.ErrClosed) {
		return true
	}

	var operationErr *net.OpError

	if errors.As(err, &operationErr) {
		if errors.Is(operationErr.Err, syscall.EPIPE) ||
			errors.Is(operationErr.Err, syscall.ECONNRESET) {
			return true
		}
	}

	message := err.Error()

	return strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "connection reset by peer")
}
