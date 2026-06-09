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
	sessions         *sync.Map
	server           *http.Server
	nextConnID       uint64
	lastPositions    atomic.Pointer[map[string]any]
	lastEquity       atomic.Pointer[map[string]any]
	lastDecisionTree atomic.Pointer[map[string]any]
	lastDumps        atomic.Pointer[map[string]any]
	lastGauges        sync.Map
	lastBalances     atomic.Pointer[user.Balances]
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
		ctx:      ctx,
		cancel:   cancel,
		pool:     pool,
		bus:      internal.NewBus(ctx, pool, []string{"kraken:private"}, []string{"ui"}),
		clients:  &sync.Map{},
		sessions: &sync.Map{},
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

	hub.sessions.Range(func(key, value any) bool {
		connID, ok := key.(uint64)

		if ok {
			hub.detachClient(connID)
		}

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

	if attachErr := hub.attachClient(connID, conn); attachErr != nil {
		errnie.Error(attachErr)
		hub.clients.Delete(connID)
		errnie.Error(conn.Close())

		return
	}

	hub.subscribeBalances()
	hub.replayBalances(conn)
}

func (hub *Hub) subscribeBalances() {
	params := &user.BalanceParams{
		Channel:  "balances",
		Snapshot: true,
	}

	token, tokenErr := types.NewToken(hub.ctx)

	if errnie.Error(tokenErr) == nil {
		params.Token = token
	}

	errnie.Error(hub.bus.Send("kraken:private", "balances", types.KrakenMessage{
		Method: "subscribe",
		Params: params,
		ReqID:  time.Now().UnixNano(),
	}))
}

func (hub *Hub) rememberBalances(value any) {
	balances, ok := value.(user.Balances)

	if !ok {
		return
	}

	stored := balances
	hub.lastBalances.Store(&stored)
}

func (hub *Hub) replayBalances(conn *websocket.Conn) {
	snapshot := hub.lastBalances.Load()

	if snapshot == nil {
		return
	}

	if err := conn.WriteJSON(snapshot); err != nil {
		errnie.Error(err)
	}
}

/*
Tick drains qpool into per-client outbound disruptors so the bus subscriber never
waits on browser pressure. Saturated client rings drop the oldest frame.
*/
func (hub *Hub) Tick() error {
	for {
		row, err := hub.bus.Receive("ui")

		if errnie.Error(err) != nil || row == nil {
			continue
		}

		if row.Type == "balances" {
			hub.rememberBalances(row.Value)
		}

		hub.publishToClients(row.Value)
	}
}
