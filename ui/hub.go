package ui

import (
	"context"
	"fmt"
	"net"
	neturl "net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/lf"
)

var UI *Hub

func init() {
	UI = NewHub(context.Background())
	UI.Run()
}

type Hub struct {
	*runtime.System
	ctx        context.Context
	app        *fiber.App
	listenAddr string
	routes     *Routes
	conn       atomic.Pointer[websocket.Conn]
	queue      *lf.Queue[[]byte]
	writeMu    sync.Mutex
	connected  atomic.Bool
	callbacks  []func()
}

/*
NewHub constructs the dashboard hub from its queue-backed system boundaries.
*/
func NewHub(ctx context.Context) *Hub {
	hub := &Hub{
		ctx:        ctx,
		listenAddr: system.Cfg.UI.Addr,
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  system.Cfg.UI.WebSocket.MaxMessageBytes,
			WriteBufferSize: system.Cfg.UI.WebSocket.MaxMessageBytes,
		}),
		queue:     lf.NewQueue[[]byte](),
		callbacks: make([]func(), 0),
	}

	hub.routes = NewRoutes(hub)
	hub.System = runtime.NewSystem(ctx, "hub")

	// The dashboard is a separate origin from the hub (vite dev server on
	// :3000 vs. the hub on :8765). Permit loopback origins on any port
	// so a locally-served dashboard can always read it without opening CORS to
	// arbitrary remote origins.
	hub.app.Use(cors.New(cors.Config{
		AllowOriginsFunc: func(origin string) bool {
			parsed, err := neturl.Parse(origin)

			if err != nil {
				return false
			}

			host := parsed.Hostname()

			return host == "localhost" || host == "127.0.0.1" || host == "::1"
		},
		AllowMethods:        []string{"GET", "POST", "HEAD", "OPTIONS"},
		AllowHeaders:        []string{"*"},
		AllowPrivateNetwork: true,
	}))

	hub.routes.Register()

	return hub
}

/*
wsHandler serves the dashboard socket.

The hub's fasthttp server recycles a connection the moment its handler
returns (the hijacked-socket keep flag this websocket helper tries to set
only applies to net.Listener hijacks, not fasthttp's own hijack path).
The handler must therefore block until the dashboard actually goes away:
reading until error does exactly that, so the browser's own connection
lifetime — not the server's — decides when frames stop.
*/
func (hub *Hub) wsHandler() fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		defer func() {
			conn.Close()

			if hub.conn.CompareAndSwap(conn, (*websocket.Conn)(nil)) {
				hub.connected.Store(false)
			}
		}()

		hub.writeMu.Lock()

		old := hub.conn.Swap(conn)

		if old != nil {
			old.Close()
		}

		// A fresh connection starts from an empty queue: stale frames for
		// the previous connection would carry wire sequence numbers the new
		// dashboard reads as duplicates and drop.
		for {
			if _, ok := hub.queue.Dequeue(); !ok {
				break
			}
		}

		// Attached: publishers may produce again, and the replay below
		// (Hello) flows through Write.
		hub.connected.Store(true)

		for _, callback := range hub.callbacks {
			callback()
		}

		/*
			This drain is the connection's writer: it lives on the handler
			goroutine, so it is retired the instant the handler returns and no
			drained frame can outlive its socket. It exits quietly once the
			handler swaps in a replacement connection.
		*/
		go func() {
			for {
				if hub.ctx.Err() != nil {
					return
				}

				hub.writeMu.Lock()

				if hub.conn.Load() != conn {
					hub.writeMu.Unlock()
					return
				}

				p, ok := hub.queue.Dequeue()

				if !ok {
					hub.writeMu.Unlock()
					time.Sleep(5 * time.Millisecond)
					continue
				}

				err := conn.WriteMessage(websocket.BinaryMessage, p)

				hub.writeMu.Unlock()

				if err != nil {
					conn.Close()
					return
				}
			}
		}()

		hub.writeMu.Unlock()

		for {
			select {
			case <-hub.ctx.Done():
				return
			default:
			}

			_, payload, err := conn.Conn.ReadMessage()

			if err != nil {
				// The dashboard is gone; the deferred teardown above
				// retires this connection and the drain with it.
				return
			}

			if err := hub.handleCommand(payload); err != nil {
				continue
			}
		}
	}, websocket.Config{
		Origins: []string{"*"},
	})
}

/*
Write fans one serialized Value out to every connected dashboard client
without blocking: no client, or a full client queue, drops the frame.
p must be a fresh buffer (io.Copy(hub, value) hands over value.Bytes()).
*/
func (hub *Hub) Write(p []byte) (n int, err error) {
	if hub.connected.Load() {
		hub.queue.Enqueue(p)
	}

	return len(p), nil
}

/*
Active reports whether a dashboard is attached. Publishers that build a frame
per tick check it to skip the serialization entirely when nobody is listening.
*/
func (hub *Hub) Active() bool {
	return hub.connected.Load()
}

/*
RegisterCallback registers a callback to be called when a new websocket connection is established.
*/
func (hub *Hub) RegisterCallback(callback func()) {
	hub.callbacks = append(hub.callbacks, callback)
}

/*
handleCommand dispatches one inbound JSON command from the dashboard socket.
A command that does not decode, or names no known type, is logged with its
raw text and returned as an error.
*/
func (hub *Hub) handleCommand(payload []byte) error {
	var request struct {
		Type      string `json:"type"`
		Symbol    string `json:"symbol"`
		Route     string `json:"route"`
		Run       string `json:"run"`
		Class     string `json:"class"`
		At        string `json:"at"`
		CaptureID int64  `json:"captureId"`
		Prefix    string `json:"prefix"`
		Query     string `json:"query"`
	}

	if err := sonic.Unmarshal(payload, &request); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[hub] malformed dashboard command %s", string(payload)),
			err,
		))
	}

	switch request.Type {
	case "focus":
		types.SetFocus(request.Symbol)
	case "route":
		types.SetRoute(request.Route)
	case "position.exit":
	case "hindsight":
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"[hub] unknown dashboard command type %q in %q",
				request.Type, payload,
			),
			nil,
		))
	}

	return nil
}

/*
App exposes the underlying Fiber application for registering domain services.
*/
func (hub *Hub) App() *fiber.App {
	return hub.app
}

/*
Run binds the dashboard listener before returning and only then transitions
the hub READY, so nothing upstream writes to a hub that cannot serve yet.
*/
func (hub *Hub) Run() error {
	listener, err := net.Listen("tcp", hub.listenAddr)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("[hub] listen on %s failed", hub.listenAddr),
			err,
		))
	}

	go func() {
		if err := hub.app.Listener(listener); err != nil {
			hub.Error(errnie.Err(
				errnie.IO,
				fmt.Sprintf("[hub] serve on %s failed", hub.listenAddr),
				err,
			))
		}
	}()

	hub.Transition(runtime.READY)
	return nil
}
