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
	"github.com/theapemachine/symm/config"
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

	for _, channel := range []string{
		"wallet",
		"ohlc",
		"subscriptions",
		"confidence",
		"causal_graph",
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

	_ = client.conn.WriteJSON(map[string]any{
		"event": "hello",
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
	})

	go func() {
		defer func() {
			client.closed.Store(true)
			hub.clients.Delete(client)
			_ = client.conn.Close()
		}()

		for hub.ctx.Err() == nil {
			if _, _, err := client.conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

func (hub *Hub) writePump() {
	for hub.ctx.Err() == nil {
		for _, sub := range hub.subscriptions {
			select {
			case value, open := <-sub.Incoming:
				if !open || value == nil {
					continue
				}

				hub.pool.Schedule("ui:broadcast", func(ctx context.Context) (any, error) {
					hub.clients.Range(func(key, _ any) bool {
						client, ok := key.(*wsClient)

						if !ok || client.closed.Load() {
							return true
						}

						_ = client.conn.WriteJSON(value.Value)
						return true
					})

					return nil, nil
				})
			default:
			}
		}

		if delay := config.System.RescoreEvery; delay > 0 {
			time.Sleep(delay)
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
