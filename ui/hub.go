package ui

import (
	"context"
	"fmt"
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
Hub drains the shared ui broadcast group and writes frames to websocket clients.
Producers Send on the group; writePump delivers asynchronously so Kraken websocket
frames are not blocked when the orchestrator is in Measure.
*/
type Hub struct {
	ctx            context.Context
	cancel         context.CancelFunc
	clients        sync.Map
	ui             *qpool.BroadcastGroup
	uiSubscription *qpool.Subscriber
	runID          string
}

/*
NewHub subscribes to the shared ui broadcast group created on the pool.
*/
func NewHub(
	ctx context.Context,
	ui *qpool.BroadcastGroup,
) (*Hub, error) {
	if ui == nil {
		return nil, fmt.Errorf("ui hub requires ui broadcast group")
	}

	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		ctx:    ctx,
		cancel: cancel,
		ui:     ui,
		runID:  time.Now().UTC().Format("20060102T150405Z"),
	}

	hub.uiSubscription = ui.Subscribe("ui", 4096)

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

	hub.clients.Store(client, struct{}{})

	_ = client.conn.WriteJSON(map[string]any{
		"event":  "hello",
		"ts":     time.Now().UTC().Format(time.RFC3339Nano),
		"run_id": hub.runID,
	})

	hub.readPump(client)
}

func (hub *Hub) writePump() {
	for {
		select {
		case <-hub.ctx.Done():
			return
		case value := <-hub.uiSubscription.Incoming:
			if value == nil {
				continue
			}

			hub.broadcastToClients(value)
		}
	}
}

func (hub *Hub) broadcastToClients(value *qpool.QValue[any]) {
	hub.clients.Range(func(key, _ any) bool {
		client, ok := key.(*wsClient)

		if !ok || client.closed.Load() {
			return true
		}

		switch value.Value.(type) {
		case map[string]any:
			_ = client.conn.WriteJSON(value.Value)
		case []map[string]any:
			_ = client.conn.WriteJSON(value.Value)
		}

		return true
	})
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

		hub.ui.Send(&qpool.QValue[any]{
			Value: payload,
		})
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
