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
	"github.com/google/uuid"
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
Hub subscribes to the ui broadcast group and forwards frames to all connected
dashboard websocket clients.
*/
type Hub struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q[any]
	bus          *internal.Bus
	clients      sync.Map
	server       *http.Server
	lastBalances atomic.Pointer[user.Balances]
}

type frontendClient struct {
	hub      *Hub
	conn     atomic.Pointer[websocket.Conn]
	client   string
	outbound chan []byte
	closed   atomic.Bool
}

func startFrontendClient(hub *Hub, conn *websocket.Conn) *frontendClient {
	bufferSize := viper.GetInt("ui.outbound_buffer")

	if bufferSize <= 0 {
		bufferSize = 512
	}

	client := &frontendClient{
		hub:      hub,
		client:   uuid.NewString(),
		outbound: make(chan []byte, bufferSize),
	}

	client.conn.Store(conn)
	go client.pump()

	return client
}

func (client *frontendClient) pump() {
	for payload := range client.outbound {
		if client.closed.Load() {
			return
		}

		conn := client.conn.Load()

		if conn == nil {
			return
		}

		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			client.hub.detachFrontend(client)
			errnie.Error(err)

			return
		}
	}
}

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
			ctx,
			pool,
			[]internal.Channel{internal.ChannelKrakenPrivate},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelUI, "ui:hub"),
			},
		),
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

	hub.clients.Range(func(key any, value any) bool {
		if client, ok := value.(*frontendClient); ok {
			hub.detachFrontend(client)
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

	client := startFrontendClient(hub, conn)

	hub.clients.Store(client.client, client)
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

	errnie.Error(hub.bus.Send(internal.ChannelKrakenPrivate, "balances", types.KrakenMessage{
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

	frame, err := WalletFrame(*snapshot)

	if err != nil {
		errnie.Error(err)

		return
	}

	client.send(frame)
}

func (hub *Hub) detachFrontend(client *frontendClient) {
	if client == nil {
		return
	}

	hub.clients.Delete(client.client)
	client.close()
}

func (client *frontendClient) send(value any) {
	if client.closed.Load() {
		return
	}

	buf, err := json.Marshal(value)

	if err != nil {
		errnie.Error(err)

		return
	}

	client.enqueue(buf)
}

func (client *frontendClient) enqueue(payload []byte) {
	if client.closed.Load() {
		return
	}

	select {
	case client.outbound <- payload:
		return
	default:
	}

	select {
	case <-client.outbound:
	default:
	}

	select {
	case client.outbound <- payload:
	default:
	}
}

func (client *frontendClient) close() {
	if client.closed.Swap(true) {
		return
	}

	close(client.outbound)

	if conn := client.conn.Swap(nil); conn != nil {
		errnie.Error(conn.Close())
	}
}

func (hub *Hub) broadcast(value any) {
	payload, err := json.Marshal(value)

	if err != nil {
		errnie.Error(err)

		return
	}

	hub.broadcastBytes(payload)
}

func (hub *Hub) broadcastBytes(payload []byte) {
	hub.clients.Range(func(key any, stored any) bool {
		if client, ok := stored.(*frontendClient); ok {
			client.enqueue(payload)
		}

		return true
	})
}

func (hub *Hub) prepareUIFrame(row *qpool.QValue[any]) (string, any, bool) {
	if row == nil {
		return "", nil, false
	}

	value := row.Value

	if row.Type == "balances" {
		hub.rememberBalances(value)

		balances, ok := value.(user.Balances)

		if !ok {
			return "", nil, false
		}

		frame, frameErr := WalletFrame(balances)

		if frameErr != nil {
			errnie.Error(frameErr)

			return "", nil, false
		}

		return coalesceKey("wallet", frame), frame, true
	}

	return coalesceKey(row.Type, value), value, true
}

func (hub *Hub) drainUI() {
	first, err := hub.bus.Receive(internal.ChannelUI)

	if errnie.Error(err) != nil || first == nil {
		return
	}

	pending := make(map[string]any)

	if key, value, ok := hub.prepareUIFrame(first); ok {
		pending[key] = value
	}

	for {
		row, pollErr := hub.bus.Poll(internal.ChannelUI)

		if pollErr != nil || row == nil {
			break
		}

		key, value, ok := hub.prepareUIFrame(row)

		if !ok {
			continue
		}

		pending[key] = value
	}

	for _, value := range pending {
		hub.broadcast(value)
	}
}

/*
Tick drains the ui bus, coalesces high-frequency telemetry, and forwards the
latest frame per key to connected frontend clients without blocking producers.
*/
func (hub *Hub) Tick() error {
	for {
		hub.drainUI()
	}
}
