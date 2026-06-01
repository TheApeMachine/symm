package ui

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

const (
	writeDeadline = 2 * time.Second
	// perClientBuffer absorbs measurement bursts. The signal layer can
	// emit dozens of frames in a single tick (one per gauge, plus mark,
	// plus prediction). 4096 gives ~10 ticks of headroom before drop
	// kicks in, which is well above the worst-case burst we've measured.
	perClientBuffer = 4096
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
) *Hub {
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

	go hub.Serve(viper.GetViper().GetString("ui.addr"))

	return hub
}

func (hub *Hub) Close() error {
	hub.cancel()
	return nil
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

	hub.clients.Store("ws", conn)
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
		case value, ok := <-hub.subscriptions["ui"].Incoming:
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

			hub.clients.Range(func(key, value any) bool {
				client := value.(*websocket.Conn)
				client.WriteJSON(out)
				return true
			})
		}
	}
}
