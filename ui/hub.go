package ui

import (
	"context"
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

	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

type wsClient struct {
	conn   *websocket.Conn
	closed atomic.Bool
}

func (client *wsClient) close() error {
	if client.closed.Swap(true) {
		return nil
	}

	return client.conn.Close()
}

func (client *wsClient) writeJSON(payload any) error {
	if client.closed.Load() {
		return websocket.ErrCloseSent
	}

	return client.conn.WriteJSON(payload)
}

/*
Hub subscribes to every broadcast group and writes payloads to websocket clients.
*/
type Hub struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q
	broadcasts    map[string]*qpool.BroadcastGroup
	subscriptions map[string]*qpool.Subscriber
	clients       *sync.Map
}

/*
NewHub subscribes to all broadcast groups on pool.
*/
func NewHub(
	ctx context.Context,
	pool *qpool.Q,
) (*Hub, error) {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		ctx:           ctx,
		cancel:        cancel,
		pool:          pool,
		broadcasts:    make(map[string]*qpool.BroadcastGroup),
		subscriptions: make(map[string]*qpool.Subscriber),
		clients:       &sync.Map{},
	}

	hub.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	hub.subscriptions["ui"] = hub.broadcasts["ui"].Subscribe("ui", 128)

	go hub.writePump(hub.subscriptions["ui"])

	return hub, errnie.Require(map[string]any{
		"ctx":           hub.ctx,
		"cancel":        hub.cancel,
		"pool":          hub.pool,
		"broadcasts":    hub.broadcasts,
		"subscriptions": hub.subscriptions,
		"clients":       hub.clients,
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

	hub.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event": "hello",
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
	}})
}

func (hub *Hub) writePump(subscription *qpool.Subscriber) {
	for {
		select {
		case <-hub.ctx.Done():
			return
		case value := <-subscription.Incoming:
			if value == nil || value.Value == nil {
				return
			}

			hub.clients.Range(func(key, _ any) bool {
				client, ok := key.(*wsClient)

				if !ok || client.closed.Load() {
					return true
				}

				if err := client.writeJSON(value.Value); err != nil {
					errnie.Error(client.close())
					return true
				}

				return true
			})
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
