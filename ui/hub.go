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
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
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
	ctx              context.Context
	cancel           context.CancelFunc
	pool             *qpool.Q
	broadcasts       map[string]*qpool.BroadcastGroup
	subscribers      map[string]*qpool.Subscriber
	clients          *sync.Map
	server           *http.Server
	nextConnID       uint64
	lastWallet       atomic.Pointer[map[string]any]
	lastPositions    atomic.Pointer[map[string]any]
	lastEquity       atomic.Pointer[map[string]any]
	lastDecisionTree atomic.Pointer[map[string]any]
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

	for _, channel := range []string{"ui"} {
		hub.broadcasts[channel] = bus.Group(pool, channel, 500*time.Millisecond)
		hub.subscribers[channel] = hub.broadcasts[channel].Subscribe(channel, 128)
	}

	addr := viper.GetViper().GetString("ui.addr")
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.handleWS)
	mux.HandleFunc("/api/dumps", hub.handleListDumps)
	mux.HandleFunc("/api/analyze", hub.handleAnalyze)

	listener := errnie.Does(func() (net.Listener, error) {
		return net.Listen("tcp", addr)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	hub.server = &http.Server{
		Handler: mux,
	}

	if listener == nil {
		// The UI port could not be bound — almost always a stale instance still
		// holding it. The dashboard is non-essential to trading, so degrade to
		// headless rather than handing a nil listener to Serve, which panics and
		// takes the whole trading process down with it.
		errnie.Error(errors.New("ui: dashboard disabled — could not bind " + addr + " (running headless)"))

		return hub
	}

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
		errnie.Error(err)
		errnie.Error(conn.Close())
		return
	}

	if w := hub.lastWallet.Load(); w != nil {
		_ = conn.WriteJSON(*w)
	}

	if positions := hub.lastPositions.Load(); positions != nil {
		_ = conn.WriteJSON(*positions)
	}

	if equity := hub.lastEquity.Load(); equity != nil {
		_ = conn.WriteJSON(*equity)
	}

	if decisionTree := hub.lastDecisionTree.Load(); decisionTree != nil {
		_ = conn.WriteJSON(*decisionTree)
	}

	connID := atomic.AddUint64(&hub.nextConnID, 1)
	hub.clients.Store(connID, conn)
}

/*
Tick drains qpool into the lossy telemetry ring and fanout to websocket clients.
from that ring so the qpool subscriber never waits on browser pressure.
*/
func (hub *Hub) Tick() error {
	for {
		select {
		case <-hub.ctx.Done():
			return hub.ctx.Err()
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

			if out["event"] == "wallet" {
				hub.lastWallet.Store(&out)
			}

			if out["event"] == "positions" {
				hub.lastPositions.Store(&out)
			}

			if out["event"] == "equity" {
				hub.lastEquity.Store(&out)
			}

			if out["chart"] == "decision_tree" {
				hub.lastDecisionTree.Store(&out)
			}

			hub.clients.Range(func(key, value any) bool {
				conn, ok := value.(*websocket.Conn)

				if !ok {
					hub.clients.Delete(key)

					return true
				}

				if err := conn.WriteJSON(out); err != nil {
					errnie.Error(err)
					errnie.Error(conn.Close())
					hub.clients.Delete(key)
				}

				return true
			})
		}
	}
}
