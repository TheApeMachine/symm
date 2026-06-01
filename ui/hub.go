package ui

import (
	"context"
	"errors"
	"fmt"
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
	server        *http.Server
	nextConnID    uint64
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

	addr := viper.GetViper().GetString("ui.addr")
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.handleWS)

	listener, err := net.Listen("tcp", addr)

	if err != nil {
		cancel()

		return nil, fmt.Errorf("ui hub: listen %s: %w", addr, err)
	}

	hub.server = &http.Server{
		Handler: mux,
	}

	go func() {
		serveErr := hub.server.Serve(listener)

		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errnie.Error(serveErr)
		}
	}()

	return hub, nil
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
		conn, ok := value.(*websocket.Conn)

		if ok {
			if err := conn.Close(); err != nil {
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
		_ = errnie.Error(err)
		return
	}

	connID := atomic.AddUint64(&hub.nextConnID, 1)
	hub.clients.Store(connID, conn)
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
				conn, ok := value.(*websocket.Conn)

				if !ok {
					hub.clients.Delete(key)

					return true
				}

				if err := conn.WriteJSON(out); err != nil {
					errnie.Error(err)
					_ = conn.Close()
					hub.clients.Delete(key)
				}

				return true
			})
		}
	}
}
