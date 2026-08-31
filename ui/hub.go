package ui

import (
	"context"
	"errors"
	"net"
	neturl "net/url"
	"strconv"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/store"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
TradeJournalSource supplies the persisted trade journal. It is the narrow slice
of the broker PositionStore the hub needs to serve GET /trades, kept as an
interface so the UI layer never depends on the broker package's concrete type.
*/
type TradeJournalSource interface {
	RecentTrades(limit int) ([]*wire.PositionT, error)
}

/*
Hub owns the dashboard websocket and broadcasts schema-tagged binary frames.
It is an ordinary Workspace stage: it registers to ChannelUI through NewHub,
and the Workspace drives every outbound write through Step. Inbound commands
arrive over the same socket and are handled directly by the connection's
handler goroutine, so there are no per-client writer or reader goroutines.
*/
type Hub struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	app        *fiber.App
	listenAddr string
	clients    *sync.Map
	store      *store.SQLite
	tradeStore TradeJournalSource
	timelines  *timelineCache
	fluid      *FluidRTC
}

type Client struct {
	conn *websocket.Conn
	mu   sync.Mutex
	// queue is the bounded publication boundary for one dashboard socket:
	// capacity one, latest-wins. Step never blocks on it; the per-client
	// sender both drains the freshest replaceable snapshot and applies the
	// disconnect policy when the socket dies.
	queue  chan []byte
	closed chan struct{}
}

/*
enqueue replaces any pending dashboard snapshot and wakes the sender without
blocking. A slow consumer therefore receives a fresher replaceable state when
it catches up, never a backlog of stale frames, and Hub.Step is never stalled
by an external browser.
*/
func (client *Client) enqueue(payload []byte) {
	if client == nil || client.queue == nil {
		return
	}

	select {
	case <-client.closed:
		return
	default:
	}

	select {
	case client.queue <- payload:
	default:
		// Latest-wins: drop the stale pending snapshot.
		select {
		case <-client.queue:
		default:
		}
		select {
		case client.queue <- payload:
		default:
		}
	}
}

/*
runSender drains the freshest pending snapshot and writes it under the client
mutex. It exits when the client closes, never leaking a goroutine.
*/
func (client *Client) runSender() {
	for {
		select {
		case <-client.closed:
			return
		case payload := <-client.queue:
			client.mu.Lock()
			err := client.conn.WriteMessage(websocket.BinaryMessage, payload)
			client.mu.Unlock()

			if err != nil {
				return
			}
		}
	}
}

/*
NewHub constructs the dashboard hub from its queue-backed system boundaries and
registers it on the workspace so live frames reach it through Step.
*/
func NewHub(ctx context.Context) *Hub {
	ctx, cancel := context.WithCancel(ctx)
	viper.SetDefault("ui.addr", "127.0.0.1:8765")
	viper.SetDefault("ui.websocket.max_message_bytes", 4*1024*1024)

	hub := &Hub{
		ctx:        ctx,
		cancel:     cancel,
		listenAddr: viper.GetString("ui.addr"),
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  4194304,
			WriteBufferSize: 4194304,
		}),
		clients:   &sync.Map{},
		timelines: newTimelineCache(),
		fluid:     NewFluidRTC(ctx, "hub"),
	}

	// The dashboard is a separate origin from the hub (vite dev server on
	// :3000 vs. the hub on :8765). The REST capture listing is fetched with a
	// plain cross-origin GET, so the browser blocks the response without an
	// Access-Control-Allow-Origin header. Permit loopback origins on any port
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
		AllowHeaders: []string{"Content-Type"},
	}))

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/trades", func(c fiber.Ctx) error {
		if hub.tradeStore == nil {
			return c.JSON([]*wire.PositionT{})
		}

		limit := parseUintQuery(c.Query("limit"))

		if limit > 2000 {
			limit = 2000
		}

		trades, err := hub.tradeStore.RecentTrades(int(limit))

		if err != nil {
			return err
		}

		if trades == nil {
			trades = []*wire.PositionT{}
		}

		return c.JSON(trades)
	})

	// Hindsight inspection reads: the capture tape and its persisted
	// historical EnvelopeStates, joined by identity for scrub-and-inspect.
	hub.app.Get("/hindsight/runs", func(c fiber.Ctx) error {
		if hub.store == nil {
			return c.JSON([]any{})
		}

		runs, err := hub.store.ListRuns()

		if err != nil {
			return err
		}

		return c.JSON(runs)
	})

	hub.app.Get("/hindsight/captures", func(c fiber.Ctx) error {
		if hub.store == nil {
			return c.JSON([]any{})
		}

		after := parseUintQuery(c.Query("after"))

		captures, err := hub.store.ListCapturesAfter(c.Query("run"), after, 0)

		if err != nil {
			return err
		}

		return c.JSON(captures)
	})

	// /hindsight/states returns every persisted historical EnvelopeState of a
	// run as the raw flatbuffer bytes, one per record, so the dashboard decodes
	// them with the same EnvelopeState class it uses for the live stream.
	hub.app.Get("/hindsight/states", func(c fiber.Ctx) error {
		if hub.store == nil {
			return c.JSON([]any{})
		}

		states, err := hub.store.ListStates(c.Query("run"))

		if err != nil {
			return err
		}

		return c.JSON(states)
	})

	// /hindsight/gaps returns every concrete capture/integrity defect recorded
	// for a run, so the UI can show why a run is not COMPLETE.
	hub.app.Get("/hindsight/gaps", func(c fiber.Ctx) error {
		if hub.store == nil {
			return c.JSON([]any{})
		}

		gaps, err := hub.store.ListGaps(c.Query("run"))

		if err != nil {
			return err
		}

		return c.JSON(gaps)
	})

	// /hindsight/envelope returns a single EnvelopeRef's full inspection record:
	// the exact CaptureIdentity, raw payload, its manifests, and its artifact
	// witnesses — fetched by identity, not by scanning a whole run.
	hub.app.Get("/hindsight/envelope", func(c fiber.Ctx) error {
		if hub.store == nil {
			return c.JSON(map[string]any{})
		}

		run := c.Query("run")
		sequence := parseUintQuery(c.Query("seq"))

		// The URL carries the run-local coordinate, which already names one
		// external input; the frame read answers with the complete identity
		// rather than requiring the caller to already hold the transport
		// fields it came here to look up.
		capture, payload, found, err := hub.store.ReadCaptureFrame(run, sequence)

		if err != nil {
			return err
		}

		if !found {
			return c.JSON(map[string]any{})
		}

		manifests, err := hub.store.ListManifestsForCapture(run, sequence)

		if err != nil {
			return err
		}

		witnesses, err := hub.store.ListWitnessesForCapture(run, sequence)

		if err != nil {
			return err
		}

		// The provenance view needs the shape of what was witnessed, not the
		// artifacts' bytes: one observe witness alone carries a serialized
		// EnvelopeState that has averaged megabytes on real runs. /hindsight/state
		// serves that payload for the exact envelope a reader asked to open.
		for index := range witnesses {
			witnesses[index].Payload = nil
		}

		return c.JSON(struct {
			Run       string                       `json:"run"`
			Sequence  uint64                       `json:"sequence"`
			Capture   store.CaptureEntry           `json:"capture"`
			Payload   []byte                       `json:"payload"`
			Manifests []hindsight.EnvelopeManifest `json:"manifests"`
			Witnesses []hindsight.ArtifactWitness  `json:"witnesses"`
		}{
			Run:       run,
			Sequence:  sequence,
			Capture:   capture,
			Payload:   payload,
			Manifests: manifests,
			Witnesses: witnesses,
		})
	})

	// /hindsight/state returns the single EnvelopeState for one exact capture
	// + ordinal, instead of shipping every state of the run.
	hub.app.Get("/hindsight/state", func(c fiber.Ctx) error {
		if hub.store == nil {
			return c.JSON(map[string]any{})
		}

		run := c.Query("run")
		sequence := parseUintQuery(c.Query("seq"))
		ordinal := parseUintQuery(c.Query("ordinal"))

		state, found, err := hub.store.ReadState(run, sequence, ordinal)

		if err != nil {
			return err
		}

		if !found {
			return c.JSON(map[string]any{})
		}

		return c.JSON(state)
	})

	// /hindsight/lifecycle returns every trading-lifecycle transition of a run,
	// correlated by decision ID.
	hub.app.Get("/hindsight/lifecycle", func(c fiber.Ctx) error {
		if hub.store == nil {
			return c.JSON([]any{})
		}

		events, err := hub.store.ListLifecycleEvents(c.Query("run"))

		if err != nil {
			return err
		}

		return c.JSON(events)
	})

	hub.registerTimeline()

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		key := uuid.NewString()
		client := &Client{
			conn:   conn,
			queue:  make(chan []byte, 1),
			closed: make(chan struct{}),
		}
		hub.clients.Store(key, client)

		go client.runSender()

		defer func() {
			close(client.closed)
			hub.clients.Delete(key)
			conn.Conn.Close()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				messageType, payload, err := conn.Conn.ReadMessage()

				if err != nil {
					return
				}

				if messageType != websocket.TextMessage {
					continue
				}

				hub.handleCommand(payload)
			}
		}
	}, websocket.Config{
		Origins: []string{"*"},
	}))

	hub.registerFluidWebRTC()

	return hub
}

/*
Step is the Ticker/Trade/Level3 Observe boundary's UI publisher (README §12,
§18): it witnesses the Envelope committed by the previous barrier and
broadcasts it as-is — the same exported state the backend is carrying,
converted to FlatBuffers and never reshaped into a curated per-consumer
frame. It never mutates the Envelope and its return value is discarded by the
Observe HandlerGroup, so a disconnected or slow UI cannot change what the
system believes.
*/
func (hub *Hub) Step(envelope *types.Envelope) *types.Envelope {
	payload := envelope.EncodeWebsocket()

	hub.clients.Range(func(key, value any) bool {
		client, valid := value.(*Client)

		if !valid || client == nil {
			return true
		}

		client.enqueue(payload)

		return true
	})

	// The three high-volume, loss-tolerant payloads leave the websocket and
	// ride their own WebRTC channels instead. Each is encoded only when a
	// connected viewer actually owns that channel: observer serialization
	// work is demand-driven, so a run with no resonance/diagnostics/manifold
	// viewer spends none of it. Observer snapshots are replaceable; durable
	// historical truth lives in Hindsight/raw capture; external viewers can
	// never backpressure trading computation.
	if envelope.Manifold != nil && hub.fluid.HasChannel(types.ManifoldChannel) {
		if err := hub.fluid.Publish(envelope.Manifold); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"hub: publish manifold frame",
				err,
			))
		}
	}

	if envelope.Resonance != nil && hub.fluid.HasChannel(types.ResonanceChannel) {
		if err := hub.fluid.PublishResonance(envelope); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"hub: publish resonance frame",
				err,
			))
		}
	}

	if len(envelope.Boundaries) > 0 && hub.fluid.HasChannel(types.DiagnosticsChannel) {
		if err := hub.fluid.PublishDiagnostics(envelope); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"hub: publish diagnostics frame",
				err,
			))
		}
	}

	return envelope
}

func (hub *Hub) Name() string { return "hub" }
func (hub *Hub) Error() error { return hub.err }

/*
SetHindsightStore attaches the capture store so the Hindsight inspection reads
(runs, captures, persisted states) can answer without the live path. It is set
after boot because the store opens after the hub in cmd/root.go.
*/
func (hub *Hub) SetHindsightStore(store *store.SQLite) {
	if hub == nil {
		return
	}

	hub.store = store
}

/*
SetTradeStore attaches the broker's trade journal so GET /trades can serve the
persisted position_trades table. It is set after boot because the position
store opens after the hub in cmd/root.go.
*/
func (hub *Hub) SetTradeStore(source TradeJournalSource) {
	if hub == nil {
		return
	}

	hub.tradeStore = source
}

/*
parseUintQuery parses a uint64 query parameter, returning 0 on absence or
malformation so a missing selector reads as "no match" rather than crashing the
handler.
*/
func parseUintQuery(raw string) uint64 {
	value, err := strconv.ParseUint(raw, 10, 64)

	if err != nil {
		return 0
	}

	return value
}

/*
handleCommand dispatches one inbound JSON command from the dashboard socket.
*/
func (hub *Hub) handleCommand(payload []byte) {
	var request struct {
		Type      string `json:"type"`
		Symbol    string `json:"symbol"`
		At        string `json:"at"`
		CaptureID int64  `json:"captureId"`
	}

	if err := sonic.Unmarshal(payload, &request); err != nil {
		return
	}

	switch request.Type {
	case "focus":
		types.SetFocus(request.Symbol)
	case "position.exit":
	}
}

/*
Run listens for dashboard clients on the configured address.

It honours ui.addr, which NewHub already reads: the field existed but the
listener ignored it, so configuring the address had no effect. The default
(127.0.0.1:8765) is loopback-only, which is what the CORS policy above already
assumes the dashboard to be.
*/
func (hub *Hub) Run() error {
	address := hub.listenAddr

	if address == "" {
		address = "127.0.0.1:8765"
	}

	return hub.app.Listen(address)
}

/*
Close shuts down the HTTP server, cancels clients, and waits for ingress drain.
*/
func (hub *Hub) Close() error {
	var err error

	hub.cancel()

	if hub.fluid != nil {
		err = hub.fluid.Close()
	}

	if hub.app != nil {
		err = errors.Join(err, hub.app.Shutdown())
	}

	if errors.Is(err, net.ErrClosed) {
		return nil
	}

	return err
}
