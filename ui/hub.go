package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	neturl "net/url"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

type Hub struct {
	*runtime.System
	ctx        context.Context
	app        *fiber.App
	listenAddr string
	routes     *Routes
	clients    sync.Map // *websocket.Conn -> *client
	replayer   Replayer
	detector   Detector
	retained   atomic.Pointer[[]byte]
}

/*
Retained is a writer for the one frame every client must hold whatever
time it connects (the loaded CognitionTree): each Write is broadcast like
Write and kept, and a client that connects later receives the latest one
first. p must be one whole serialized Value.
*/
func (hub *Hub) Retained() io.Writer { return retainedWriter{hub: hub} }

type retainedWriter struct{ hub *Hub }

/*
join registers a new client, the retained frame queued first.
*/
func (hub *Hub) join(key any, current *client) {
	// The queue is empty: the retained frame is always the first one.
	if retained := hub.retained.Load(); retained != nil {
		current.frames <- *retained
	}

	hub.clients.Store(key, current)
}

func (writer retainedWriter) Write(p []byte) (int, error) {
	frame := slices.Clone(p)
	writer.hub.retained.Store(&frame)
	return writer.hub.Write(frame)
}

/*
Replayer streams one stored run to one client (hindsight.Service): it
writes whole serialized Values to w until done, ctx is canceled or w fails.
*/
type Replayer interface {
	Stream(ctx context.Context, w io.Writer, run uint64, symbol string) error
}

/*
Detector streams what detect stored to one client (hindsight.Service, the
Historical runs tab): run 0 the labelled epochs, a run its excursions, a run
with symbol, class and C tick that excursion's trades. It writes whole
serialized Values to w until done, ctx is canceled or w fails.
*/
type Detector interface {
	Detections(ctx context.Context, w io.Writer, run uint64, symbol, class string, cTick uint64) error
}

/*
client is one dashboard socket. Its frames queue has one writer goroutine.
Broadcast frames (Hub.Write) never wait on it; a stream written through the
client itself (io.Writer) waits for room, so a stream is never dropped, and
ends when the socket closes. A new stream of a kind cancels the one of
that kind in flight. Kinds run side by side. The hindsight index and the
open symbol tape are different kinds, so opening a symbol leaves the index
naming the others. A new run's index cancels the tape from the run it
replaces. Switching the open symbol cancels only that tape.
*/
type client struct {
	frames  chan []byte
	done    chan struct{}
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

var errClientClosed = errors.New("[hub] dashboard client closed")

func (client *client) Write(p []byte) (int, error) {
	frame := slices.Clone(p)

	select {
	case client.frames <- frame:
		return len(p), nil
	case <-client.done:
		return 0, errClientClosed
	}
}

/*
replay starts the run index or one symbol tape. The index keeps naming
symbols while a tape is open. A new index replaces the previous index and
cancels the tape. A new tape replaces only the previous tape.
*/
func (client *client) replay(parent context.Context, replayer Replayer, run uint64, symbol string) {
	kind := "hindsight-tape"

	if symbol == "" {
		kind = "hindsight-index"
		client.cancelStream("hindsight-tape")
	}

	client.stream(parent, kind, fmt.Sprintf("run %d %q", run, symbol), func(ctx context.Context) error {
		return replayer.Stream(ctx, client, run, symbol)
	})
}

func (client *client) cancelStream(kind string) {
	client.mu.Lock()
	defer client.mu.Unlock()

	if client.cancels == nil {
		return
	}

	if cancel := client.cancels[kind]; cancel != nil {
		cancel()
		delete(client.cancels, kind)
	}
}

/*
stream cancels the client's stream of kind in flight, if any, and runs this
one; what names it in the log.
*/
func (client *client) stream(parent context.Context, kind, what string, run func(context.Context) error) {
	client.mu.Lock()
	defer client.mu.Unlock()

	if client.cancels == nil {
		client.cancels = make(map[string]context.CancelFunc)
	}

	if cancel := client.cancels[kind]; cancel != nil {
		cancel()
	}

	ctx, cancel := context.WithCancel(parent)
	client.cancels[kind] = cancel

	go func() {
		defer cancel()

		select {
		case <-client.done:
			cancel()
		case <-ctx.Done():
		}
	}()

	go func() {
		err := run(ctx)

		if ctx.Err() != nil {
			errnie.Info(fmt.Sprintf("[hub] %s stream of %s ended: %v", kind, what, context.Cause(ctx)))
			return
		}

		if err != nil {
			errnie.Error(errnie.Err(errnie.IO, fmt.Sprintf("[hub] %s stream of %s", kind, what), err))
		}

		cancel()
	}()
}

/*
Detect sets the detector serving the "detections" command. It is set before
Run; without one the command is an error.
*/
func (hub *Hub) Detect(detector Detector) {
	hub.detector = detector
}

/*
Replay sets the replayer serving the "hindsight" command. It is set before
Run; without one the command is an error.
*/
func (hub *Hub) Replay(replayer Replayer) {
	hub.replayer = replayer
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
clientQueue bounds the frames buffered per dashboard client; when a client
falls behind, new frames are dropped so the pipeline never waits on it.
*/
const clientQueue = 256

func (hub *Hub) wsHandler(shard int) fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		current := &client{frames: make(chan []byte, clientQueue), done: make(chan struct{})}
		hub.join(conn, current)
		errnie.Info(fmt.Sprintf("[hub] frontend websocket shard %d connected", shard))

		defer func() {
			hub.clients.Delete(conn)
			// done, not frames, is closed: a broadcast or a replay may still
			// hold the client, and sending on a closed channel panics.
			close(current.done)
			errnie.Info(fmt.Sprintf("[hub] frontend websocket shard %d disconnected", shard))

			if err := conn.Conn.Close(); err != nil {
				errnie.Error(err)
			}
		}()

		// The only writer on this connection.
		go func() {
			for {
				select {
				case frame := <-current.frames:
					if err := conn.Conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
						return
					}
				case <-current.done:
					return
				}
			}
		}()

		// Block on the read loop: returning closes the connection.
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

			// A bad command is logged by handleCommand; the socket stays
			// open for the next one.
			if err := hub.handleCommand(current, payload); err != nil {
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
	hub.clients.Range(func(_, entry any) bool {
		current := entry.(*client)

		select {
		case current.frames <- p:
		case <-current.done:
		default:
		}

		return true
	})

	return len(p), nil
}

/*
handleCommand dispatches one inbound JSON command from the dashboard socket.
A command that does not decode, or names no known type, is logged with its
raw text and returned as an error.
*/
func (hub *Hub) handleCommand(from *client, payload []byte) error {
	var request struct {
		Type      string `json:"type"`
		Symbol    string `json:"symbol"`
		Route     string `json:"route"`
		Run       string `json:"run"`
		Class     string `json:"class"`
		At        string `json:"at"`
		CaptureID int64  `json:"captureId"`
	}

	if err := sonic.Unmarshal(payload, &request); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, fmt.Sprintf("[hub] malformed dashboard command %q", payload), err,
		))
	}

	switch request.Type {
	case "focus":
		types.SetFocus(request.Symbol)
	case "route":
		types.SetRoute(request.Route)
	case "hindsight":
		// The run is a decimal string: an epoch exceeds what a JSON number
		// carries exactly in the browser.
		run, err := strconv.ParseUint(request.Run, 10, 64)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation, fmt.Sprintf("[hub] hindsight command run %q is not an epoch", request.Run), err,
			))
		}

		if hub.replayer == nil || from == nil {
			return errnie.Error(errnie.Err(
				errnie.ServiceUnavailable, "[hub] hindsight command with no replayer or no client: "+string(payload), nil,
			))
		}

		from.replay(hub.ctx, hub.replayer, run, request.Symbol)
	case "detections":
		// run "" lists the labelled epochs; run and symbol name one
		// excursion, its class and its C tick (at) too. Epochs and ticks
		// are decimal strings, beyond what a JSON number carries exactly.
		var run, cTick uint64
		var err error

		if request.Run != "" {
			if run, err = strconv.ParseUint(request.Run, 10, 64); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Validation, fmt.Sprintf("[hub] detections command run %q is not an epoch", request.Run), err,
				))
			}
		}

		if request.Symbol != "" {
			if cTick, err = strconv.ParseUint(request.At, 10, 64); err != nil || run == 0 || request.Class == "" {
				return errnie.Error(errnie.Err(
					errnie.Validation, fmt.Sprintf("[hub] detections command for a symbol needs a run, a class and a C tick (at): %s", payload), err,
				))
			}
		}

		if hub.detector == nil || from == nil {
			return errnie.Error(errnie.Err(
				errnie.ServiceUnavailable, "[hub] detections command with no detector or no client: "+string(payload), nil,
			))
		}

		kind := "detection tape"

		switch {
		case run == 0:
			kind = "detection runs"
		case request.Symbol == "":
			kind = "detection excursions"
		}

		from.stream(hub.ctx, kind, string(payload), func(ctx context.Context) error {
			return hub.detector.Detections(ctx, from, run, request.Symbol, request.Class, cTick)
		})
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation, fmt.Sprintf("[hub] unknown dashboard command type %q in %q", request.Type, payload), nil,
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
	if hub.listenAddr == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "[hub] ui.addr is not configured", nil))
	}

	listener, err := net.Listen("tcp", hub.listenAddr)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "[hub] listen on "+hub.listenAddr+" failed", err))
	}

	go func() {
		if err := hub.app.Listener(listener); err != nil {
			hub.Error(errnie.Err(errnie.IO, "[hub] serve on "+hub.listenAddr+" failed", err))
		}
	}()

	hub.Transition(runtime.READY)
	return nil
}
