package ui

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
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
Hub subscribes to the ui broadcast group and forwards frames to the dashboard
websocket client.
*/
type Hub struct {
	ctx    context.Context
	cancel context.CancelFunc
	bus    *internal.Bus
	client atomic.Pointer[websocket.Conn]
	server *http.Server
}

func NewHub(
	ctx context.Context,
	pool *qpool.Q[any],
) *Hub {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		ctx:    ctx,
		cancel: cancel,
		bus: internal.NewBus(
			ctx,
			pool,
			nil,
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelUI, "ui:hub"),
			},
		),
	}

	addr := viper.GetString("ui.addr")
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

	if conn := hub.client.Swap(nil); conn != nil {
		errnie.Error(conn.Close())
	}

	hub.cancel()

	return hub.bus.Close()
}

func (hub *Hub) handleWS(writer http.ResponseWriter, request *http.Request) {
	conn, err := wsUpgrader.Upgrade(writer, request, nil)

	if err != nil {
		errnie.Error(err)
		return
	}

	if previous := hub.client.Swap(conn); previous != nil {
		errnie.Error(previous.Close())
	}

	hello := map[string]any{
		"event": "hello",
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
	}

	if err := conn.WriteJSON(hello); err != nil {
		errnie.Error(err)
		hub.detach(conn)

		return
	}
}

func (hub *Hub) detach(conn *websocket.Conn) {
	if hub.client.CompareAndSwap(conn, nil) {
		errnie.Error(conn.Close())
	}
}

func (hub *Hub) write(value any) {
	conn := hub.client.Load()

	if conn == nil {
		return
	}

	payload, err := json.Marshal(value)

	if err != nil {
		errnie.Error(err)

		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		hub.detach(conn)
		errnie.Error(err)
	}
}

func (hub *Hub) Tick() error {
	for {
		message, err := hub.bus.Receive(internal.ChannelUI)

		if internal.ReportError(err) != nil {
			return err
		}

		if message == nil {
			continue
		}

		hub.write(message.Value)
	}
}
