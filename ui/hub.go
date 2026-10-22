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
	stopWriter chan struct{}
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

func (hub *Hub) wsHandler() fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		// One drain goroutine owns the hub-wide queue: a new connection
		// takes over the current writer instead of starting a racer. The
		// drain generation lets the previous writer exit as soon as it is
		// replaced, so a stale goroutine can never dequeue a frame it
		// cannot deliver.
		drain := make(chan struct{})

		hub.writeMu.Lock()

		if hub.stopWriter != nil {
			close(hub.stopWriter)
		}

		hub.stopWriter = drain

		old := hub.conn.Load()
		hub.conn.Store(conn)

		if old != nil {
			old.Close()
		}

		for _, callback := range hub.callbacks {
			callback()
		}

		hub.writeMu.Unlock()

		go func() {
			for {
				select {
				case <-hub.ctx.Done():
					return
				case <-drain:
					return
				default:
					hub.writeMu.Lock()

					p, ok := hub.queue.Dequeue()

					if !ok {
						hub.writeMu.Unlock()
						time.Sleep(10 * time.Millisecond)
						continue
					}

					conn := hub.conn.Load()

					if conn == nil {
						// No dashboard attached: drop the frame — it is
						// live state, the next one supersedes it.
						hub.writeMu.Unlock()
						continue
					}

					err := conn.WriteMessage(websocket.BinaryMessage, p)

					hub.writeMu.Unlock()

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.IO,
							"[hub] write to websocket failed",
							err,
						))
					}
				}
			}
		}()

		for {
			select {
			case <-hub.ctx.Done():
				return
			default:
			}

			_, payload, err := conn.Conn.ReadMessage()

			if err != nil {
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
	hub.queue.Enqueue(p)
	return len(p), nil
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
