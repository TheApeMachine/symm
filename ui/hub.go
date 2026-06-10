package ui

import (
	"context"
	"encoding/json"
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
Hub subscribes to the ui broadcast group and forwards frames to one dashboard
websocket client. Balance snapshots are retained so reconnects replay the last
known state. The frontend never sends frames upstream.
*/
type Hub struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q[any]
	bus          *internal.Bus
	frontend     atomic.Pointer[frontendClient]
	server       *http.Server
	lastBalances atomic.Pointer[user.Balances]
}

type frontendClient struct {
	hub  *Hub
	conn *websocket.Conn
	mu   sync.Mutex
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
		bus:    internal.NewBus(ctx, pool, []string{"kraken:private"}, []string{"ui"}),
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

	if client := hub.frontend.Load(); client != nil {
		hub.detachFrontend(client)
	}

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

	client := &frontendClient{
		hub:  hub,
		conn: conn,
	}

	if previous := hub.frontend.Swap(client); previous != nil {
		previous.close()
	}

	hub.subscribeBalances()
	hub.replayBalances(client)
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

func (hub *Hub) replayBalances(client *frontendClient) {
	snapshot := hub.lastBalances.Load()

	if snapshot == nil {
		return
	}

	client.send(WalletFrame(*snapshot))
}

func (hub *Hub) detachFrontend(client *frontendClient) {
	if client == nil {
		return
	}

	if hub.frontend.CompareAndSwap(client, nil) {
		client.close()

		return
	}

	client.close()
}

func (client *frontendClient) send(value any) {
	client.mu.Lock()
	defer client.mu.Unlock()

	if client.conn == nil {
		return
	}

	if _, err := json.Marshal(value); err != nil {
		errnie.Error(err)

		return
	}

	if err := client.conn.WriteJSON(value); err != nil {
		client.hub.detachFrontend(client)
		errnie.Error(err)
	}
}

func (client *frontendClient) close() {
	client.mu.Lock()
	defer client.mu.Unlock()

	if client.conn != nil {
		errnie.Error(client.conn.Close())
		client.conn = nil
	}
}

/*
Tick drains the ui bus and forwards each frame to the connected frontend client.
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

		client := hub.frontend.Load()

		if client == nil {
			continue
		}

		value := row.Value

		if row.Type == "balances" {
			if balances, ok := value.(user.Balances); ok {
				value = WalletFrame(balances)
			}
		}

		client.send(value)
	}
}
