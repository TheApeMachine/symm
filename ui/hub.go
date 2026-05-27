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

	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

type wsClient struct {
	conn   *websocket.Conn
	closed atomic.Bool
}

func (client *wsClient) close() error {
	client.closed.Store(true)

	return client.conn.Close()
}

func (client *wsClient) writeJSON(payload any) error {
	if client.closed.Load() {
		return websocket.ErrCloseSent
	}

	if client.closed.Load() {
		return websocket.ErrCloseSent
	}

	return client.conn.WriteJSON(payload)
}

/*
Hub subscribes to every broadcast group and writes payloads to websocket clients.
*/
type Hub struct {
	ctx             context.Context
	cancel          context.CancelFunc
	pool            *qpool.Q
	broadcasts      map[string]*qpool.BroadcastGroup
	subscriptions   map[string]*qpool.Subscriber
	clients         *sync.Map
	walletSnap      atomic.Value
	confidenceSnaps sync.Map
	fieldSnap       atomic.Value
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

	for _, channel := range []string{
		"wallet",
		"ohlc",
		"confidence",
		"feedback",
		"executions",
		"exits",
		"orders",
		"ui",
	} {
		hub.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		hub.subscriptions[channel] = hub.broadcasts[channel].Subscribe("ui:"+channel, 128)
	}

	go hub.writePump()

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

	hub.sendSnapshots(client)

	go func() {
		defer func() {
			hub.clients.Delete(client)
			errnie.Error(client.close())
		}()

		for errnie.Error(hub.ctx.Err()) == nil {
			if _, _, err := client.conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

func (hub *Hub) remember(channel string, payload any) {
	if payload == nil {
		return
	}

	switch channel {
	case "wallet":
		hub.walletSnap.Store(payload)
	case "confidence":
		row, ok := payload.(map[string]any)

		if !ok {
			return
		}

		source, ok := row["source"].(string)

		if !ok || source == "" {
			return
		}

		hub.confidenceSnaps.Store(source, payload)
	case "ui":
		row, ok := payload.(map[string]any)

		if !ok {
			return
		}

		if row["event"] == "field_snapshot" {
			hub.fieldSnap.Store(payload)
		}
	}
}

func (hub *Hub) sendSnapshots(client *wsClient) {
	if wallet := hub.walletSnap.Load(); wallet != nil {
		hub.broadcasts["ui"].Send(&qpool.QValue[any]{Value: wallet})
	}

	hub.confidenceSnaps.Range(func(_, value any) bool {
		hub.broadcasts["ui"].Send(&qpool.QValue[any]{Value: value})

		return true
	})

	if field := hub.fieldSnap.Load(); field != nil {
		hub.broadcasts["ui"].Send(&qpool.QValue[any]{Value: field})
	}
}

func (hub *Hub) writePump() {
	errnie.Info("starting ui hub pump")

	for hub.ctx.Err() == nil {
		wrote := false

		select {
		case value, open := <-hub.subscriptions["ui"].Incoming:
			hub.write("ui", value, open)
			wrote = true
		case value, open := <-hub.subscriptions["feedback"].Incoming:
			hub.write("feedback", value, open)
			wrote = true
		case value, open := <-hub.subscriptions["executions"].Incoming:
			hub.write("executions", value, open)
			wrote = true
		case value, open := <-hub.subscriptions["exits"].Incoming:
			hub.write("exits", value, open)
			wrote = true
		case value, open := <-hub.subscriptions["orders"].Incoming:
			hub.write("orders", value, open)
			wrote = true
		case value, open := <-hub.subscriptions["ohlc"].Incoming:
			hub.write("ohlc", value, open)
			wrote = true
		case value, open := <-hub.subscriptions["confidence"].Incoming:
			hub.write("confidence", value, open)
			wrote = true
		case value, open := <-hub.subscriptions["wallet"].Incoming:
			hub.write("wallet", value, open)
			wrote = true
		}

		if !wrote {
			time.Sleep(time.Millisecond)
		}
	}
}

func (hub *Hub) write(channel string, value *qpool.QValue[any], open bool) {
	if !open || value == nil {
		return
	}

	hub.remember(channel, value.Value)

	hub.clients.Range(func(key, _ any) bool {
		client, ok := key.(*wsClient)

		if !ok || client.closed.Load() {
			return true
		}

		if err := client.writeJSON(value.Value); err != nil {
			errnie.Error(client.close())
			return false
		}

		return true
	})
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
