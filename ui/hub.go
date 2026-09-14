package ui

import (
	"context"
	"errors"
	"net"
	neturl "net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"fmt"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/signal"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/wf"
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
LearningSource supplies serialized learning state FlatBuffers.
*/
type LearningSource interface {
	MarshalFlatbuffer(focus string) []byte
}

type StreamableLearningSource interface {
	MarshalFlatbufferWith(focus string, fn func([]byte) error) error
}

/*
Hub owns the dashboard websocket and broadcasts schema-tagged binary frames.
It is an ordinary Workspace stage: it registers to ChannelUI through NewHub,
and the Workspace drives every outbound write through Step. Inbound commands
arrive over the same socket and are handled directly by the connection's
handler goroutine, so there are no per-client writer or reader goroutines.
*/
type Hub struct {
	physics          sensorium.PhysicsMonitor
	ctx              context.Context
	cancel           context.CancelFunc
	err              error
	app              *fiber.App
	listenAddr       string
	frontend         *websocket.Conn
	frontendMu       sync.Mutex
	store            *tables.Catalog
	tradeStore       TradeJournalSource
	learningSource   LearningSource
	exitHandler      func(symbol string)
	fluid            *FluidRTC
	learningInterval time.Duration
	lastLearning     time.Time
}

/*
NewHub constructs the dashboard hub from its queue-backed system boundaries and
registers it on the workspace so live frames reach it through Step.
*/
func NewHub(ctx context.Context) *Hub {
	viper.SetDefault("ui.websocket.learning_interval", "250ms")
	ctx, cancel := context.WithCancel(ctx)
	viper.SetDefault("ui.addr", "127.0.0.1:8765")
	viper.SetDefault("ui.websocket.max_message_bytes", 4*1024*1024)

	hub := &Hub{learningInterval: viper.GetDuration("ui.websocket.learning_interval"),
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
		fluid: NewFluidRTC(ctx, "hub"),
	}

	// The dashboard is a separate origin from the hub (vite dev server on
	// :3000 vs. the hub on :8765). The REST capture listing is fetched with a
	// plain cross-origin GET, so the browser blocks the response without an
	// Access-Control-Allow-Origin header. Permit loopback origins on any port
	// so a locally-served dashboard can always read it without opening CORS to
	// arbitrary remote origins.
	hub.app.Use(func(c fiber.Ctx) error {
		c.Set("Access-Control-Allow-Private-Network", "true")
		return c.Next()
	})

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

	hub.registerPhysics()

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

	// Hindsight inspection projection reads
	hub.app.Get("/hindsight/metric-map", func(c fiber.Ctx) error {
		return c.JSON(signal.Semantics())
	})

	hub.app.Get("/hindsight/runs", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		runs, err := hub.store.Runs(hub.ctx)

		if err != nil {
			return err
		}

		return c.JSON(runs)
	})

	hub.app.Get("/hindsight/timeline", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		run := c.Query("run")

		if run == "" {
			return fiber.NewError(fiber.StatusBadRequest, "run is required")
		}

		query := tables.TimelineQuery{
			Run:        run,
			Symbol:     c.Query("symbol"),
			Coordinate: c.Query("coordinate"),
			Axis:       c.Query("axis"),
			Buckets:    int(parseUintQuery(c.Query("buckets"))),
			From:       parseInt64Query(c.Query("from")),
			To:         parseInt64Query(c.Query("to")),
			Symbols:    c.Query("symbols") == "1",
		}

		timeline, err := hub.store.Timeline(hub.ctx, query)

		if err != nil {
			return err
		}

		return c.JSON(timeline)
	})

	hub.app.Get("/hindsight/lifecycle", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("run"))
		events, err := hub.store.Lifecycle(hub.ctx, epoch)

		if err != nil {
			return err
		}

		return c.JSON(events)
	})

	hub.app.Get("/hindsight/captures", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("run"))
		after := parseInt64Query(c.Query("after"))
		captures, err := hub.store.Captures(hub.ctx, epoch, after)

		if err != nil {
			return err
		}

		return c.JSON(captures)
	})

	hub.app.Get("/hindsight/envelope", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("run"))
		seq := parseInt64Query(c.Query("seq"))
		envelope, err := hub.store.EnvelopeAt(hub.ctx, epoch, seq)

		if err != nil {
			return fiber.ErrNotFound
		}

		return c.JSON(envelope)
	})

	hub.app.Get("/hindsight/state", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		return fiber.ErrNotFound
	})

	hub.app.Get("/hindsight/states", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		return c.JSON([]tables.HindsightState{})
	})

	hub.app.Get("/hindsight/resident", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("run"))
		symbol := c.Query("symbol")
		seq := parseInt64Query(c.Query("seq"))
		budget := int(parseUintQuery(c.Query("budget")))

		resident, err := hub.store.ResidentAt(hub.ctx, epoch, symbol, seq, budget)

		if err != nil {
			return err
		}

		return c.JSON(resident)
	})

	hub.app.Get("/hindsight/gaps", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		return c.JSON([]tables.HindsightGap{})
	})

	// Hindsight canonical table reads
	hub.app.Get("/hindsight/measurements", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("epoch"))
		after := parseInt64Query(c.Query("after"))
		rows, err := hub.store.Measurements(hub.ctx, epoch, after)

		if err != nil {
			return err
		}

		return c.JSON(rows)
	})

	hub.app.Get("/hindsight/spot_level3", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("epoch"))
		after := parseInt64Query(c.Query("after"))
		rows, err := hub.store.SpotLevel3(hub.ctx, epoch, after)

		if err != nil {
			return err
		}

		return c.JSON(rows)
	})

	hub.app.Get("/hindsight/spot_ticker", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("epoch"))
		after := parseInt64Query(c.Query("after"))
		rows, err := hub.store.SpotTicker(hub.ctx, epoch, after)

		if err != nil {
			return err
		}

		return c.JSON(rows)
	})

	hub.app.Get("/hindsight/spot_trade", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("epoch"))
		after := parseInt64Query(c.Query("after"))
		rows, err := hub.store.SpotTrade(hub.ctx, epoch, after)

		if err != nil {
			return err
		}

		return c.JSON(rows)
	})

	hub.app.Get("/hindsight/futures_ticker", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("epoch"))
		after := parseInt64Query(c.Query("after"))
		rows, err := hub.store.FuturesTicker(hub.ctx, epoch, after)

		if err != nil {
			return err
		}

		return c.JSON(rows)
	})

	hub.app.Get("/hindsight/futures_trade", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("epoch"))
		after := parseInt64Query(c.Query("after"))
		rows, err := hub.store.FuturesTrade(hub.ctx, epoch, after)

		if err != nil {
			return err
		}

		return c.JSON(rows)
	})

	hub.app.Get("/hindsight/executions", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("epoch"))
		after := parseInt64Query(c.Query("after"))
		rows, err := hub.store.Executions(hub.ctx, epoch, after)

		if err != nil {
			return err
		}

		return c.JSON(rows)
	})

	hub.app.Get("/hindsight/models", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("epoch"))
		after := parseInt64Query(c.Query("after"))
		rows, err := hub.store.Models(hub.ctx, epoch, after)

		if err != nil {
			return err
		}

		return c.JSON(rows)
	})

	hub.app.Get("/hindsight/grids", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("epoch"))
		after := parseInt64Query(c.Query("after"))
		rows, err := hub.store.Grids(hub.ctx, epoch, after)

		if err != nil {
			return err
		}

		return c.JSON(rows)
	})

	hub.app.Get("/hindsight/positions", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("epoch"))
		after := parseInt64Query(c.Query("after"))
		rows, err := hub.store.Positions(hub.ctx, epoch, after)

		if err != nil {
			return err
		}

		return c.JSON(rows)
	})

	hub.app.Get("/hindsight/decisions", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("epoch"))
		after := parseInt64Query(c.Query("after"))
		rows, err := hub.store.Decisions(hub.ctx, epoch, after)

		if err != nil {
			return err
		}

		return c.JSON(rows)
	})

	hub.app.Get("/hindsight/outcomes", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(c.Query("epoch"))
		after := parseInt64Query(c.Query("after"))
		rows, err := hub.store.Outcomes(hub.ctx, epoch, after)

		if err != nil {
			return err
		}

		return c.JSON(rows)
	})
	hub.registerWorkbench()

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		hub.frontendMu.Lock()
		hub.frontend = conn
		hub.frontendMu.Unlock()
		errnie.Info("hub: frontend websocket connected")

		hub.broadcastLearning()

		defer func() {
			hub.frontendMu.Lock()

			if hub.frontend == conn {
				hub.frontend = nil
			}

			hub.frontendMu.Unlock()
			errnie.Info("hub: frontend websocket disconnected")

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
/*
Step satisfies nmruntime.Node[*data.Measurement[float64]].
*/
func (hub *Hub) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if measurement == nil {
		return nil
	}

	hub.writeFrontend(measurement)
	return measurement
}

/*
Consume attaches the hub to a runtime stream channel and starts the goroutine
that serves it.
*/
func (hub *Hub) Consume(measurements <-chan *data.Measurement[float64]) {
	go hub.publish(measurements)
}

/*
Drain runs the asynchronous micro-batching drain loop against a wait-free SPSC
ring buffer. It drains available measurements and flushes them to the connected
frontend in consolidated FlatBuffer batches.
*/
func (hub *Hub) Drain(ring *wf.RingBuffer[*data.Measurement[float64]]) {
	if ring == nil {
		return
	}

	ticker := time.NewTicker(4 * time.Millisecond)
	defer ticker.Stop()

	batch := make([]*data.Measurement[float64], 0, 64)

	for {
		select {
		case <-hub.ctx.Done():
			return
		case <-ticker.C:
		}

		if !hub.hasFrontend() {
			for {
				if _, ok := ring.Get(); !ok {
					break
				}
			}

			continue
		}

		for {
			measurement, ok := ring.Get()

			if !ok {
				break
			}

			if hub.isWireAllowed(measurement) {
				batch = append(batch, measurement)
			}

			if len(batch) >= 64 {
				hub.writeMeasurements(batch)
				batch = batch[:0]
			}
		}

		if len(batch) > 0 {
			hub.writeMeasurements(batch)
			batch = batch[:0]
		}

		if hub.learningSource != nil && time.Since(hub.lastLearning) >= hub.learningInterval {
			hub.lastLearning = time.Now()
			hub.broadcastLearning()
		}
	}
}

func (hub *Hub) hasFrontend() bool {
	hub.frontendMu.Lock()
	defer hub.frontendMu.Unlock()

	return hub.frontend != nil
}

/*
publish serves the incoming stream of measurements to the frontend.
*/
func (hub *Hub) publish(measurements <-chan *data.Measurement[float64]) {
	for {
		var measurement *data.Measurement[float64]

		select {
		case <-hub.ctx.Done():
			return
		case measurement = <-measurements:
		}

		if measurement == nil {
			continue
		}

		hub.writeFrontend(measurement)
	}
}

/*
PublishMeasurement writes one measurement to the dashboard socket.
*/
func (hub *Hub) PublishMeasurement(measurement *data.Measurement[float64]) {
	hub.writeFrontend(measurement)
}

/*
PublishMeasurements writes a batch of measurements to the dashboard socket.
*/
func (hub *Hub) PublishMeasurements(measurements []*data.Measurement[float64]) {
	hub.writeMeasurements(measurements)
}

/*
IsRawMarketData reports whether a measurement carries raw spot or futures market data
(ticker, trade, level3/book) that should not be broadcast over the dashboard websocket.
*/
func IsRawMarketData(measurement *data.Measurement[float64]) bool {
	if measurement == nil {
		return true
	}

	source := strings.ToLower(measurement.Source)

	if source == "websocket" || source == "public" || source == "private" || source == "spot" || source == "futures" {
		return true
	}

	if slices.Contains(types.SignalSourceStrings, source) || slices.Contains(types.LogicSourceStrings, source) ||
		strings.HasPrefix(source, "pumpdump") || strings.HasPrefix(source, "toxicity") ||
		strings.HasPrefix(source, "depthflow") || strings.HasPrefix(source, "morphology") ||
		strings.HasPrefix(source, "derivatives") || strings.HasPrefix(source, "cognition") ||
		strings.HasPrefix(source, "training") {
		return false
	}

	if measurement.Provenance != nil {
		channel := strings.ToLower(measurement.Provenance["channel"])

		if channel == "ticker" || channel == "trade" || channel == "level3" || channel == "book" ||
			strings.HasPrefix(channel, "futures.") {
			return true
		}
	}

	return false
}

/*
IsAllowedTelemetry reports whether a measurement is permitted for dashboard telemetry broadcast.
*/
func IsAllowedTelemetry(measurement *data.Measurement[float64]) bool {
	return !IsRawMarketData(measurement)
}

/*
isWireAllowed reports whether a measurement is permitted for dashboard telemetry broadcast
given the current focus symbol. Raw market data is rejected, training/system metrics are
permitted, and symbol-tagged metrics must match the active focus gate.
*/
func (hub *Hub) isWireAllowed(measurement *data.Measurement[float64]) bool {
	if measurement == nil || IsRawMarketData(measurement) {
		return false
	}

	if strings.HasPrefix(strings.ToLower(measurement.Source), "training") {
		return true
	}

	return types.Allows(measurement.Label)
}

/*
writeFrontend encodes one measurement as a MeasurementsFrame and writes it to the dashboard socket.
*/
func (hub *Hub) writeFrontend(measurement *data.Measurement[float64]) {
	if !hub.isWireAllowed(measurement) {
		return
	}

	hub.frontendMu.Lock()
	defer hub.frontendMu.Unlock()

	if hub.frontend == nil {
		return
	}

	_ = types.EncodeMeasurementsFrameWith([]*data.Measurement[float64]{measurement}, func(payload []byte) error {
		if len(payload) == 0 {
			return nil
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

		return nil
	})
}

/*
writeMeasurements encodes a batch of measurements as a MeasurementsFrame and writes it to the dashboard socket.
*/
func (hub *Hub) writeMeasurements(measurements []*data.Measurement[float64]) {
	if len(measurements) == 0 {
		return
	}

	hub.frontendMu.Lock()
	defer hub.frontendMu.Unlock()

	if hub.frontend == nil {
		return
	}

	filtered := make([]*data.Measurement[float64], 0, len(measurements))

	for _, measurement := range measurements {
		if !hub.isWireAllowed(measurement) {
			continue
		}

		filtered = append(filtered, measurement)
	}

	if len(filtered) == 0 {
		return
	}

	_ = types.EncodeMeasurementsFrameWith(filtered, func(payload []byte) error {
		if len(payload) == 0 {
			return nil
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

		return nil
	})
}

func (hub *Hub) broadcastLearning() {
	if hub.learningSource == nil {
		return
	}

	focus := types.Focus()

	if streamable, ok := hub.learningSource.(StreamableLearningSource); ok {
		_ = streamable.MarshalFlatbufferWith(focus, func(payload []byte) error {
			hub.writeLearning(payload)
			return nil
		})

		return
	}

	if payload := hub.learningSource.MarshalFlatbuffer(focus); len(payload) > 0 {
		hub.writeLearning(payload)
	}
}

/*
writeLearning encodes a learning state snapshot and writes it to the dashboard socket.
*/
func (hub *Hub) writeLearning(payload []byte) {
	if len(payload) == 0 {
		return
	}

	hub.frontendMu.Lock()
	defer hub.frontendMu.Unlock()

	if hub.frontend == nil {
		return
	}

	if err := hub.frontend.WriteMessage(
		websocket.BinaryMessage, payload,
	); err != nil {
		failed := hub.frontend
		hub.frontend = nil
		errnie.Warn(fmt.Sprintf("hub: websocket write learning message failed; detaching client: %v", err))

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
	return hub.fluid.Wants(types.ManifoldChannel) || hub.physics.WantsSnapshot()
}

/*
PublishManifold fans one advance's resident particles and fields to the manifold
viewers. It is called from the manifold solver's own advance goroutine, never
from the ingress path.
*/
func (hub *Hub) PublishManifold(state *types.ManifoldState) {
	if state == nil {
		return
	}

	snapshot, err := sensorium.NewPhysicsSnapshot(
		state.Version,
		state.At,
		state.State.N,
		state.Reading,
	)

	if err == nil {
		err = hub.physics.Observe(snapshot)
	}

	if err != nil {
		hub.physics.Reject(err)
		errnie.Error(errnie.Err(errnie.Internal, "hub: invalid physical health", err))
	}

	if !hub.fluid.Wants(types.ManifoldChannel) {
		return
	}

	if err := hub.fluid.Publish(state); err != nil {
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
func (hub *Hub) PublishResonance(artifact *types.ResonanceArtifact) {
	if artifact == nil || !hub.fluid.Wants(types.ResonanceChannel) {
		return
	}

	if err := hub.fluid.PublishResonance(artifact); err != nil {
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
func (hub *Hub) SetHindsightStore(store *tables.Catalog) {
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
SetLearningSource attaches the learning source so the dashboard socket can broadcast
the resident learning state periodically.
*/
func (hub *Hub) SetLearningSource(source LearningSource) {
	if hub == nil {
		return
	}

	hub.learningSource = source
}

/*
SetExitHandler attaches the handler for manual position exit commands from the UI.
*/
func (hub *Hub) SetExitHandler(handler func(symbol string)) {
	if hub == nil {
		return
	}

	hub.exitHandler = handler
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

func parseInt64Query(raw string) int64 {
	value, err := strconv.ParseInt(raw, 10, 64)

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
		hub.broadcastLearning()
	case "position.exit":
		if hub.exitHandler != nil && request.Symbol != "" {
			hub.exitHandler(request.Symbol)
		}
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
