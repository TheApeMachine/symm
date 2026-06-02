package ui

import (
	"context"
	"errors"
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
)

const (
	uiHubSubscriberID = "ui/hub:ui"
	uiResyncChannel   = "ui:resync"
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

	hub.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 500*time.Millisecond)
	hub.subscribers["ui"] = hub.broadcasts["ui"].Subscribe(uiHubSubscriberID, 128)
	hub.broadcasts[uiResyncChannel] = pool.CreateBroadcastGroup(uiResyncChannel, 10*time.Millisecond)

	addr := viper.GetViper().GetString("ui.addr")
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.handleWS)

	listener := errnie.Does(func() (net.Listener, error) {
		return net.Listen("tcp", addr)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	hub.server = &http.Server{
		Handler: mux,
	}

	activate.Boot("ui/hub listening on " + addr)

	go func() {
		serveErr := hub.server.Serve(listener)

		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errnie.Error(serveErr)
		}
	}()

	return hub
}

func (hub *Hub) Close() error {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if hub.server != nil {
		if err := hub.server.Shutdown(shutdownCtx); err != nil {
			errnie.Error(err)
		}
	}

	hub.clients.Range(func(key, value any) bool {
		conn, ok := value.(*websocket.Conn)

		if ok {
			if err := conn.Close(); err != nil {
				errnie.Error(err)
			}
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
		_ = errnie.Error(err)
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

			if value == nil || value.Value == nil {
				continue
			}

			var (
				out map[string]any
				k   bool
			)

			if out, k = value.Value.(map[string]any); !k {
				continue
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
	}
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
		errnie.Error(writeErr)
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
