package ui

import (
	"context"
	"fmt"
	"net"
	neturl "net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/internal/disconnect"
	"github.com/theapemachine/symm/store"
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
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	app        *fiber.App
	listenAddr string
	routes     *Routes
	queue      *lf.Queue[[]byte]
	conn       atomic.Pointer[websocket.Conn]
	connected  atomic.Bool
	callbacks  []func()
	commit     atomic.Value
	store      *store.Client
	exiter     Exiter
	streams    map[string]context.CancelFunc
	streamGen  map[string]uint64
	nextStream uint64
}

/*
UseStore replaces the hub object store. Call after config load so ListPrefix
reads the same bucket the model writes (ui init runs before Viper).
*/
func (hub *Hub) UseStore(client *store.Client) {
	if hub == nil || client == nil {
		return
	}

	hub.store = client
}

/*
Exiter closes a dashboard-commanded lot on the desk.
*/
type Exiter interface {
	Exit(symbol string, virtual bool) error
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
		queue:     lf.NewQueue[[]byte](),
		callbacks: make([]func(), 0),
		store:     store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
	}

	hub.commit.Store(CommitFunc(func([]byte) ([]byte, error) {
		return nil, errnie.Err(errnie.NotAcceptable, "[hindsight] commit is not attached", nil)
	}))
	hub.streams = make(map[string]context.CancelFunc)
	hub.streamGen = make(map[string]uint64)

	hub.routes = NewRoutes(hub)

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

		// This drain is the connection's writer: it lives on the handler
		// goroutine, so it is retired the instant the handler returns and no
		// drained frame can outlive its socket. It exits quietly once the
		// handler swaps in a replacement connection.
		go func() {
			for {
				if hub.ctx.Err() != nil {
					return
				}

				if hub.conn.Load() != conn {
					return
				}

				p, ok := hub.queue.Dequeue()

				if !ok {
					time.Sleep(5 * time.Millisecond)
					continue
				}

				maxBytes := system.Cfg.UI.WebSocket.MaxMessageBytes
				if maxBytes > 0 && len(p) > maxBytes {
					errnie.Warn(fmt.Sprintf(
						"[hub] dropping frame (%d bytes > max %d)",
						len(p), maxBytes,
					))
					continue
				}

				err := conn.WriteMessage(websocket.BinaryMessage, p)

				if err != nil {
					hub.retireConnection(conn, err, len(p))
					return
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

func (hub *Hub) retireConnection(conn *websocket.Conn, writeErr error, frameBytes int) {
	if hub.conn.Load() != conn {
		return
	}

	if writeErr != nil && !disconnect.Expected(writeErr) {
		errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("[hub] websocket write failed (%d bytes)", frameBytes),
			writeErr,
		))
	}

	_ = conn.Close()

	if hub.conn.CompareAndSwap(conn, nil) {
		hub.connected.Store(false)
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
HandleCommit registers the function that stores a labelled training sample.
*/
/*
Exiter registers the desk serving position.exit commands.
*/
func (hub *Hub) Exiter(exiter Exiter) {
	hub.exiter = exiter
}

func (hub *Hub) cancelStream(kind string) {
	if cancel := hub.streams[kind]; cancel != nil {
		cancel()
		delete(hub.streams, kind)
	}
}

func (hub *Hub) startStream(kind string, run func(context.Context)) {
	if cancel := hub.streams[kind]; cancel != nil {
		cancel()
	}

	ctx, cancel := context.WithCancel(hub.ctx)
	hub.nextStream++
	gen := hub.nextStream
	hub.streams[kind] = cancel
	hub.streamGen[kind] = gen

	go func() {
		run(ctx)
		if hub.streamGen[kind] == gen {
			delete(hub.streams, kind)
			delete(hub.streamGen, kind)
		}
	}()
}

func (hub *Hub) streamPrefix(kind, prefix string, done func()) {
	if hub.store == nil || prefix == "" {
		return
	}

	hub.startStream(kind, func(ctx context.Context) {
		for value := range hub.store.Stream(ctx, prefix) {
			if value == nil {
				continue
			}

			hub.Write(value.Bytes())
		}

		if ctx.Err() != nil {
			return
		}

		if done != nil {
			done()
		}
	})
}

func (hub *Hub) HandleCommit(fn CommitFunc) {
	if fn == nil {
		fn = func([]byte) ([]byte, error) {
			return nil, errnie.Err(errnie.NotAcceptable, "[hindsight] commit is not attached", nil)
		}
	}

	hub.commit.Store(fn)
}

/*
ListPrefix lists object keys under prefix in the hub store and writes one s3
frame. Each layer is the downloaded object; the index suffix after the frame
prefix is the S3 key (see data.Value.Append).
*/
func (hub *Hub) ListPrefix(prefix string) {
	if hub.store == nil || prefix == "" {
		return
	}

	value := data.NewValue("s3", "prefix", prefix)

	for key := range hub.store.ListKeys(prefix) {
		out := hub.store.Download(hub.ctx, key)
		if out == nil {
			continue
		}

		value.AppendBytes(out.Bytes(), key)
	}

	hub.Write(value.Bytes())
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
		Virtual   bool   `json:"virtual"`
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
		if hub.exiter == nil {
			return errnie.Error(errnie.Err(
				errnie.ServiceUnavailable,
				"[hub] position.exit command with no desk: "+string(payload),
				nil,
			))
		}

		if request.Symbol == "" {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[hub] position.exit command needs a symbol: "+string(payload),
				nil,
			))
		}

		if err := hub.exiter.Exit(request.Symbol, request.Virtual); err != nil {
			return errnie.Error(err)
		}
	case "hindsight":
	case "s3":
		if request.Prefix == "" {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("[hub] s3 command needs a prefix: %s", string(payload)),
				nil,
			))
		}

		if hub.store == nil {
			return errnie.Error(errnie.Err(
				errnie.ServiceUnavailable,
				fmt.Sprintf("[hub] s3 command with no store: %s", string(payload)),
				nil,
			))
		}

		hub.cancelStream("s3")

		// One aggregated frame for large prefix listings (trie books, closed lots).
		if request.Prefix == "model/path/" ||
			strings.HasPrefix(request.Prefix, "position/closed") ||
			strings.HasPrefix(request.Prefix, "virtual/closed") {
			hub.ListPrefix(request.Prefix)
			break
		}

		hub.streamPrefix("s3", request.Prefix, nil)
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
			errnie.Error(errnie.Err(
				errnie.IO,
				fmt.Sprintf("[hub] serve on %s failed", hub.listenAddr),
				err,
			))
		}
	}()

	return nil
}
