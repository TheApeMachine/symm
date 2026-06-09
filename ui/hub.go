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
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
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
	pool             *qpool.Q[any]
	bus              *internal.Bus
	clients          *sync.Map
	server           *http.Server
	nextConnID       uint64
	lastPositions    atomic.Pointer[map[string]any]
	lastEquity       atomic.Pointer[map[string]any]
	lastDecisionTree atomic.Pointer[map[string]any]
	lastDumps        atomic.Pointer[map[string]any]
	lastGauges       sync.Map
}

/*
NewHub subscribes to all broadcast groups on pool.
*/
func NewHub(
	ctx context.Context,
	pool *qpool.Q[any],
) *Hub {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		bus: internal.NewBus(
			ctx, pool, []string{"kraken:private"}, []string{"ui"},
		),
		clients: &sync.Map{},
	}

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

	hub.subscribeBalances()

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
		client, ok := value.(*websocket.Conn)

		if ok {
			if err := client.Close(); err != nil {
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
		errnie.Error(err)
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

	connID := atomic.AddUint64(&hub.nextConnID, 1)
	hub.clients.Store(connID, conn)
}

func (hub *Hub) subscribeBalances() {
	frame, err := types.NewKrakenMessage(
		"subscribe",
		user.BalanceParams{
			Channel:  "balances",
			Snapshot: true,
		},
		time.Now().UnixNano(),
	)

	if errnie.Error(err) != nil {
		return
	}

	errnie.Error(hub.bus.Send("kraken:private", "balances", frame))
}

/*
Tick drains qpool into the lossy telemetry ring and fanout to websocket clients.
from that ring so the qpool subscriber never waits on browser pressure.
*/
func (hub *Hub) Tick() error {
	for {
		row, err := hub.bus.Receive("ui")

		if errnie.Error(err) != nil || row == nil {
			continue
		}

		hub.clients.Range(func(key, stored any) bool {
			client, ok := stored.(*websocket.Conn)

			if !ok {
				hub.clients.Delete(key)
				return true
			}

			if err := client.WriteJSON(row.Value); err != nil {
				errnie.Error(err)
				hub.clients.Delete(key)
				return true
			}

			return true
		})
	}
}
