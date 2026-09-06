package ui

import (
	"context"
	"errors"
	"fmt"
	"net"
	neturl "net/url"
	"strconv"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
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
	frontend   *websocket.Conn
	frontendMu sync.Mutex
	store      *store.SQLite
	tradeStore TradeJournalSource
	timelines  *timelineCache
	fluid      *FluidRTC
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

		trades, err := hub.tradeStore.RecentTrades(int(
			min(parseUintQuery(c.Query("limit")), 2000),
		))

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
		hub.frontendMu.Lock()
		hub.frontend = conn
		hub.frontendMu.Unlock()

		defer func() {
			hub.frontendMu.Lock()

			if hub.frontend == conn {
				hub.frontend = nil
			}

			hub.frontendMu.Unlock()

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
Consume attaches the hub to a runtime Sink's channel and starts the goroutine
that serves it.

The hub is not a ring stage. Everything it does with an envelope — encoding the
websocket frame, publishing the boundary trace — is publication work, and a
publisher mounted as a Node runs that work on the ring's own goroutine at
ingress rate. It reads from the ring instead, on one goroutine that owns the
frontend connection and every publication decision outright.
*/
func (hub *Hub) Consume(envelopes <-chan *types.Envelope) {
	go hub.publish(envelopes)
}

/*
publish serves the ring's envelopes: the websocket frame first, then the
boundary trace if a viewer's diagnostics channel is ready for another one.

One goroutine does both because both are the same kind of work — encoding a
replaceable observation for whoever is watching — and serializing them here is
what keeps either from being paid on the ring. Envelopes the sink dropped were
dropped because this goroutine was busy, which is the correct answer for a live
view: it shows the present, not a backlog.
*/
func (hub *Hub) publish(envelopes <-chan *types.Envelope) {
	for {
		var envelope *types.Envelope

		select {
		case <-hub.ctx.Done():
			return
		case envelope = <-envelopes:
		}

		if envelope == nil {
			continue
		}

		hub.writeFrontend(envelope)

		if !hub.fluid.Wants(types.DiagnosticsChannel) {
			continue
		}

		if err := hub.fluid.PublishDiagnostics(envelope); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"hub: publish diagnostics frame",
				err,
			))
		}
	}
}

/*
writeFrontend encodes one envelope and writes it to the dashboard socket.

The connection is resolved and written under the same lock the /ws handler uses
to install and detach clients — the one thing here that genuinely has two
goroutines: this publisher and a connecting or vanishing browser. A failed
write detaches and closes that client immediately; the reader handler then
exits without clearing any replacement connection.
*/
func (hub *Hub) writeFrontend(envelope *types.Envelope) {
	hub.frontendMu.Lock()
	defer hub.frontendMu.Unlock()

	if hub.frontend == nil {
		return
	}

	payload := envelope.EncodeWebsocket()

	if len(payload) == 0 {
		return
	}

	if err := hub.frontend.WriteMessage(
		websocket.BinaryMessage, payload,
	); err != nil {
		failed := hub.frontend
		hub.frontend = nil
		errnie.Warn(fmt.Sprintf("hub: websocket write message failed; detaching client: %v", err))
		if err := failed.Conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errnie.Warn(fmt.Sprintf("hub: failed client close: %v", err))
		}
	}
}

/*
WantsManifold reports that a viewer is watching the resident field and its
channel is ready for another frame. The manifold advance asks before it
materializes a snapshot, so an unwatched run never pays for a field readout.
*/
func (hub *Hub) WantsManifold() bool {
	return hub.fluid.Wants(types.ManifoldChannel)
}

/*
PublishManifold fans one advance's resident particles and fields to the manifold
viewers. It is called from the manifold solver's own advance goroutine, never
from the ingress path.
*/
func (hub *Hub) PublishManifold(envelope *types.Envelope) {
	if envelope == nil || envelope.Manifold == nil {
		return
	}

	if err := hub.fluid.Publish(envelope.Manifold); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"hub: publish manifold frame",
			err,
		))
	}
}

/*
PublishResonance synchronously observes producer-owned resonance state before
the resonance Workload advances its coder to the next ticker.
*/
func (hub *Hub) PublishResonance(envelope *types.Envelope) {
	if envelope == nil || envelope.Resonance == nil ||
		!hub.fluid.Wants(types.ResonanceChannel) {
		return
	}

	if err := hub.fluid.PublishResonance(envelope); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"hub: publish resonance frame",
			err,
		))
	}
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
