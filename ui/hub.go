package ui

import (
	"context"
	"errors"
	"fmt"
	"net"
	neturl "net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/internal/disconnect"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

var UI *Hub

func init() {
	UI = NewHub(context.Background())
	if runningGoTest() {
		return
	}
	UI.Run()
}

func runningGoTest() bool {
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.") {
			return true
		}
	}
	return false
}

type Hub struct {
	ctx            context.Context
	cancel         context.CancelFunc
	err            error
	app            *fiber.App
	listenAddr     string
	routes         *Routes
	frameCh        chan []byte
	pumpOnce       sync.Once
	conn           atomic.Pointer[websocket.Conn]
	connected      atomic.Bool
	connectedSince atomic.Int64
	callbacks      []func()
	commit         atomic.Value
	store          *store.Client
	ownStore       bool
}

/*
UseStore wires the shared object-store client from Booter. Replaces any
hub-owned client opened at NewHub so the process keeps one S3 pool.
*/
func (hub *Hub) UseStore(client *store.Client) {
	if client == nil {
		return
	}

	if hub.ownStore && hub.store != nil && hub.store != client {
		_ = hub.store.Close()
	}

	hub.store = client
	hub.ownStore = false
}

/*
NewHub constructs the dashboard hub from its queue-backed system boundaries.
The hub opens its own S3 client immediately — the dashboard connects during
boot, before Booter.UseStore, and must already answer prefix fetches.
*/
func NewHub(ctx context.Context) *Hub {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		ctx:        ctx,
		cancel:     cancel,
		listenAddr: system.Cfg.UI.Addr,
		app: fiber.New(fiber.Config{
			JSONEncoder:     Sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  system.Cfg.UI.WebSocket.MaxMessageBytes,
			WriteBufferSize: system.Cfg.UI.WebSocket.MaxMessageBytes,
		}),
		frameCh:   make(chan []byte, system.Cfg.UI.WebSocket.QueueFrames),
		callbacks: make([]func(), 0),
		store:     store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
		ownStore:  true,
	}

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

		// Pause delivery so the drain below cannot forward leftovers onto
		// the socket we are about to attach.
		hub.connected.Store(false)
		hub.connectedSince.Store(0)

		old := hub.conn.Swap((*websocket.Conn)(nil))

		if old != nil {
			old.Close()
		}

		// Stale frames carry wire sequence numbers the new dashboard
		// reads as duplicates and drops.
		hub.drainFrames()

		hub.conn.Store(conn)
		hub.ensurePump()

		// Attached: publishers may produce again, and the replay below
		// (Hello) flows through Write. Record the attach instant so the first
		// ticks (produced while `connected` is still settling) are delivered.
		hub.connectedSince.Store(time.Now().UnixNano())
		hub.connected.Store(true)

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
				// retires this connection.
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

	if err := conn.Close(); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "[hub] websocket close failed", err,
		))
	}

	if hub.conn.CompareAndSwap(conn, nil) {
		hub.connected.Store(false)
	}
}

/*
Write enqueues one frame without blocking. A full buffer drops the oldest.
p must be a fresh buffer.
*/
func (hub *Hub) Write(p []byte) (int, error) {
	if len(p) == 0 || !hub.Active() {
		return len(p), nil
	}

	select {
	case hub.frameCh <- p:
		return len(p), nil
	default:
		select {
		case <-hub.frameCh:
		default:
		}

		select {
		case hub.frameCh <- p:
		default:
		}
	}

	return len(p), nil
}

func (hub *Hub) drainFrames() {
	for {
		select {
		case <-hub.frameCh:
		default:
			return
		}
	}
}

func (hub *Hub) ensurePump() {
	hub.pumpOnce.Do(func() {
		go hub.pump()
	})
}

/*
pump writes frames for the life of the hub. A nil conn drops the frame,
so a reconnect can drain leftovers before the next socket is attached.
*/
func (hub *Hub) pump() {
	for {
		select {
		case <-hub.ctx.Done():
			return
		case p := <-hub.frameCh:
			conn := hub.conn.Load()

			if conn == nil {
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

			err := conn.WriteMessage(websocket.TextMessage, p)

			if err != nil {
				hub.retireConnection(conn, err, len(p))
			}
		}
	}
}

/*
WriteJSON marshals one envelope object and fans it to the dashboard.
*/
func (hub *Hub) WriteJSON(frame map[string]any) {
	if frame == nil || !hub.Active() {
		return
	}

	buf, err := Sonic.Marshal(frame)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "[hub] marshal dashboard frame", err,
		))

		return
	}

	hub.Write(buf)
}

/*
Active reports whether a frame should be enqueued: a live connection, or one
established within the grace window. The window covers the sliver between a
connection being accepted and its dashboard's first read, where a concurrently
produced tick would otherwise be dropped and a freshly-attached dashboard would
show no metrics until the next symbol-bearing tick.
Publishers that build a frame per tick check it to skip serialization when
nobody is listening.
*/
func (hub *Hub) Active() bool {
	if hub.connected.Load() && hub.conn.Load() != nil {
		return true
	}

	since := hub.connectedSince.Load()

	if since == 0 {
		return false
	}

	return time.Since(time.Unix(0, since)) < 5*time.Second
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
		Virtual   bool   `json:"virtual"`
		Book      string `json:"book"`
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
	case "risk":
		var patch map[string]any
		if err := sonic.Unmarshal(payload, &patch); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[hub] malformed risk command "+string(payload),
				err,
			))
		}

		delete(patch, "type")
		system.Cfg.Risk.Apply(patch)
	case "resonance":
		var patch map[string]any
		if err := sonic.Unmarshal(payload, &patch); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[hub] malformed resonance command "+string(payload),
				err,
			))
		}

		delete(patch, "type")
		system.Cfg.Learning.ApplyResonance(patch)
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

		// Aggregated envelopes for trie books and closed lots. ListPrefix
		// warns and no-ops on unsupported prefixes.
		hub.ListPrefix(request.Prefix)
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

/*
Close drains the hub store and shuts the HTTP server.
*/
func (hub *Hub) Close() error {
	var errs error

	// Only close a store this hub created. Shared clients are owned by Booter.
	if hub.ownStore && hub.store != nil {
		if err := hub.store.Close(); err != nil {
			errs = errors.Join(errs, err)
		}
	}

	if hub.cancel != nil {
		hub.cancel()
	}

	if hub.app != nil {
		if err := hub.app.ShutdownWithTimeout(5 * time.Second); err != nil {
			errs = errors.Join(errs, errnie.Error(errnie.Err(
				errnie.IO, "[hub] shutdown failed", err,
			)))
		}
	}

	return errs
}
