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
	clients    map[*websocket.Conn]struct{}
	queue      *lf.Queue[[]byte]
	writeMu    sync.Mutex
	connected  atomic.Bool
	callbacks  []func()
	list       atomic.Value
	commit     atomic.Value
}

/*
CommitFunc answers POST /hindsight/commit. The body is the dashboard's
sample JSON. The return bytes are the JSON response.
*/
type CommitFunc func(body []byte) ([]byte, error)

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
		clients:   make(map[*websocket.Conn]struct{}),
		queue:     lf.NewQueue[[]byte](),
		callbacks: make([]func(), 0),
	}

	hub.list.Store(func(string) {})
	hub.commit.Store(CommitFunc(func([]byte) ([]byte, error) {
		return nil, errnie.Err(errnie.NotAcceptable, "[hindsight] commit is not attached", nil)
	}))
	go hub.writeLoop()

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

			hub.writeMu.Lock()
			delete(hub.clients, conn)
			if len(hub.clients) == 0 {
				hub.connected.Store(false)
			}
			hub.writeMu.Unlock()
		}()

		hub.writeMu.Lock()
		hub.clients[conn] = struct{}{}
		// Attached: publishers may produce again, and the replay below
		// (Hello) flows through Write to every attached dashboard.
		hub.connected.Store(true)
		hub.writeMu.Unlock()

		for _, callback := range hub.callbacks {
			callback()
		}

		for {
			select {
			case <-hub.ctx.Done():
				return
			default:
			}

			_, payload, err := conn.Conn.ReadMessage()

			if err != nil {
				// The dashboard is gone; the deferred teardown above
				// drops this socket. The others keep their frames.
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
writeLoop is the one writer for every dashboard socket. A frame dequeued
from the hub queue is written to each attached client. A client whose write
fails is closed and dropped; the rest stay up, so a second dashboard cannot
kick the first one off and start the reconnect fight.
*/
func (hub *Hub) writeLoop() {
	for {
		if hub.ctx.Err() != nil {
			return
		}

		hub.writeMu.Lock()
		payload, ok := hub.queue.Dequeue()
		if !ok {
			hub.writeMu.Unlock()
			time.Sleep(5 * time.Millisecond)
			continue
		}

		conns := make([]*websocket.Conn, 0, len(hub.clients))
		for conn := range hub.clients {
			conns = append(conns, conn)
		}
		hub.writeMu.Unlock()

		var failed []*websocket.Conn
		for _, conn := range conns {
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
				failed = append(failed, conn)
			}
		}

		for _, conn := range failed {
			conn.Close()
		}
	}
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
HandleList registers the function that answers a dashboard "s3" command.
The cognitive tree asks for policy/current when it opens.
*/
/*
HandleCommit registers the function that stores a labelled training sample.
*/
func (hub *Hub) HandleCommit(fn CommitFunc) {
	if fn == nil {
		fn = func([]byte) ([]byte, error) {
			return nil, errnie.Err(errnie.NotAcceptable, "[hindsight] commit is not attached", nil)
		}
	}

	hub.commit.Store(fn)
}

func (hub *Hub) HandleList(fn func(string)) {
	if fn == nil {
		fn = func(string) {}
	}

	hub.list.Store(fn)
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
	case "s3":
		if fn, ok := hub.list.Load().(func(string)); ok {
			fn(request.Prefix)
		}
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
