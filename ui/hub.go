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
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/squeel"
	"github.com/theapemachine/squeel/backend"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
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
	objects    objectSource
	client     *store.Client
	// kept is the latest frame of each dashboard state, replayed when a
	// client connects. Ticks are not kept: a key is one origin/role/scope.
	keepMu    sync.Mutex
	kept      map[string][]byte
	keptOrder []string
	// drops counts frames a client queue refused. The pipeline does not wait
	// on a slow dashboard, so a rising drop count is the dashboard falling
	// behind, not the venue.
	drops atomic.Uint64
}

/*
objectSource streams the objects under a prefix. Each object is one Write.
*/
type objectSource interface {
	Stream(ctx context.Context, prefix string, dst io.Writer) error
}

/*
retainedTreeKey is the cognition tree. It is replayed ahead of the other
kept frames so a connecting dashboard has the tree before the matches.
*/
const retainedTreeKey = "cognition/tree/all"

/*
Retained is the writer for the cognition tree. Retain keeps any other
latest frame (one sequitur grammar, one cognition match, per symbol).
A Write is broadcast and kept. A client that connects later receives
every kept frame, tree first. p must be one whole serialized Value.
*/
func (hub *Hub) Retained() io.Writer { return hub.Retain(retainedTreeKey) }

/*
Retain returns a writer that broadcasts p and keeps the latest bytes under key.
*/
func (hub *Hub) Retain(key string) io.Writer { return retainWriter{hub: hub, key: key} }

type retainWriter struct {
	hub *Hub
	key string
}

/*
join registers a new client and queues every kept frame first.
*/
func (hub *Hub) join(key any, current *client) {
	for _, frame := range hub.snapshotKept() {
		current.frames <- frame
	}

	hub.clients.Store(key, current)
}

func (hub *Hub) storeKept(key string, frame []byte) {
	if hub == nil || key == "" {
		return
	}

	hub.keepMu.Lock()
	defer hub.keepMu.Unlock()

	if hub.kept == nil {
		hub.kept = map[string][]byte{}
	}

	if _, ok := hub.kept[key]; !ok {
		hub.keptOrder = append(hub.keptOrder, key)
	}

	hub.kept[key] = frame
}

func (hub *Hub) snapshotKept() [][]byte {
	hub.keepMu.Lock()
	defer hub.keepMu.Unlock()

	out := make([][]byte, 0, len(hub.keptOrder))

	if frame, ok := hub.kept[retainedTreeKey]; ok {
		out = append(out, frame)
	}

	for _, key := range hub.keptOrder {
		if key == retainedTreeKey {
			continue
		}

		out = append(out, hub.kept[key])
	}

	return out
}

func (writer retainWriter) Write(p []byte) (int, error) {
	frame := slices.Clone(p)
	writer.hub.storeKept(writer.key, frame)
	return writer.hub.Write(frame)
}

/*
Replayer streams one run to the client that asked. Nothing registers one:
the dashboard reads stored prefixes itself.
*/
type Replayer interface {
	Stream(ctx context.Context, w io.Writer, run uint64, symbol string) error
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
Replay sets the replayer serving the "hindsight" command. It is set before
Run; without one the command is an error.
*/
func (hub *Hub) Replay(replayer Replayer) {
	hub.replayer = replayer
}

/*
NewHub constructs the dashboard hub from its queue-backed system boundaries.
*/
func NewHub(ctx context.Context, store *store.Client) *Hub {
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

	if store != nil {
		hub.objects = store
		hub.client = store
	}
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

		// The only writer on this connection. It starts before join so a
		// replay of every kept frame can drain instead of filling the queue.
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
			hub.drops.Add(1)
		}

		return true
	})

	return len(p), nil
}

/*
writeDetectionDone closes a detections stream. Epoch 0 is the run list,
an empty symbol is that epoch's excursions, and a symbol is one tape.
*/
func writeDetectionDone(w io.Writer, run uint64, symbol string) error {
	scope := symbol

	if scope == "" {
		scope = "all"
	}

	value := data.NewValue("detections", "done", scope)
	value.Epoch = run

	if symbol == "" {
		value.Scope = ""
	}

	value.Append([]float64{0, 0, 0, 0, 0}, "ticks", "symbols", "excursions", "policies", "papers")

	_, err := io.Copy(w, value)

	return err
}

/*
writePolicyIndex sends the object keys under prefix as one policy value.
The stored objects do not carry their key, and the trie is the key.
*/
func writePolicyIndex(ctx context.Context, client *store.Client, prefix string, dst io.Writer) error {
	value := data.NewValue("policy", "current", "index")
	value.Prefix = "policy/current"

	for key := range client.ListKeys(prefix) {
		if err := ctx.Err(); err != nil {
			return err
		}

		rest := strings.Trim(strings.TrimPrefix(key, "policy/current"), "/")

		if rest == "" {
			continue
		}

		value.Append([]byte{0}, rest)
	}

	if _, err := io.Copy(dst, value); err != nil {
		return err
	}

	return ctx.Err()
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
	case "hindsight":
		// The run is a decimal string: an epoch exceeds what a JSON number
		// carries exactly in the browser.
		run, err := strconv.ParseUint(request.Run, 10, 64)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("[hub] hindsight command run %s is not an epoch", request.Run),
				err,
			))
		}

		if hub.replayer == nil || from == nil {
			return errnie.Error(errnie.Err(
				errnie.ServiceUnavailable,
				fmt.Sprintf("[hub] hindsight command with no replayer or no client: %s", string(payload)),
				nil,
			))
		}

		from.replay(hub.ctx, hub.replayer, run, request.Symbol)
	case "detections":
		// The stored objects are the detections. run "" and a run list
		// detections/; a symbol names detections/<class>/<symbol>/.
		var run uint64
		var err error

		if request.Run != "" {
			if run, err = strconv.ParseUint(request.Run, 10, 64); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf("[hub] detections command run %s is not an epoch", request.Run),
					err,
				))
			}
		}

		if request.Symbol != "" {
			if _, err = strconv.ParseUint(request.At, 10, 64); err != nil || run == 0 || request.Class == "" {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf("[hub] detections command for a symbol needs a run, a class and a C tick (at): %s", string(payload)),
					err,
				))
			}
		}

		if hub.objects == nil || from == nil {
			return errnie.Error(errnie.Err(
				errnie.ServiceUnavailable,
				fmt.Sprintf("[hub] detections command with no store or no client: %s", string(payload)),
				nil,
			))
		}

		prefix := "detections/"

		if request.Symbol != "" {
			prefix = "detections/" + request.Class + "/" + request.Symbol + "/"
		}

		kind := "detection tape"

		switch {
		case run == 0:
			kind = "detection runs"
		case request.Symbol == "":
			kind = "detection excursions"
		}

		from.stream(hub.ctx, kind, prefix, func(ctx context.Context) error {
			if err := hub.objects.Stream(ctx, prefix, from); err != nil {
				return err
			}

			if ctx.Err() != nil {
				return ctx.Err()
			}

			return writeDetectionDone(from, run, request.Symbol)
		})
	case "s3":
		if request.Prefix == "" {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("[hub] s3 command needs a prefix: %s", string(payload)),
				nil,
			))
		}

		if hub.objects == nil || from == nil {
			return errnie.Error(errnie.Err(
				errnie.ServiceUnavailable,
				fmt.Sprintf("[hub] s3 command with no store or no client: %s", string(payload)),
				nil,
			))
		}

		prefix := request.Prefix

		from.stream(hub.ctx, "s3", prefix, func(ctx context.Context) error {
			if strings.HasPrefix(prefix, "policy/current") && hub.client != nil {
				return writePolicyIndex(ctx, hub.client, prefix, from)
			}

			return hub.objects.Stream(ctx, prefix, from)
		})
	case "query":
		if hub.client == nil || hub.client.S3Client == nil {
			return errnie.Error(errnie.Err(
				errnie.ServiceUnavailable,
				fmt.Sprintf("[hub] query command with no store: %s", string(payload)),
				nil,
			))
		}

		if request.Query == "" {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("[hub] query command needs a statement: %s", string(payload)),
				nil,
			))
		}

		squeel.NewStatement(request.Query, backend.NewS3(
			backend.WithS3Client(hub.client.S3Client),
		))
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
