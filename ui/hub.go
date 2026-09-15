package ui

import (
	"context"
	"errors"
	"io"
	"net"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal"
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
	*runtime.System
	uiTee            runtime.Tee[[]byte]
	physics          sensorium.PhysicsMonitor
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
func NewHub(
	ctx context.Context,
	uiTee runtime.Tee[[]byte],
) *Hub {
	viper.SetDefault("ui.websocket.learning_interval", "250ms")
	viper.SetDefault("ui.addr", "127.0.0.1:8765")
	viper.SetDefault("ui.websocket.max_message_bytes", 4*1024*1024)

	hub := &Hub{
		learningInterval: viper.GetDuration("ui.websocket.learning_interval"),
		uiTee:            uiTee,
		listenAddr:       viper.GetString("ui.addr"),
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  4194304,
			WriteBufferSize: 4194304,
		}),
		fluid: NewFluidRTC(ctx, "hub"),
	}

	closers := []io.Closer{}

	if uiTee != nil {
		closers = append(closers, uiTee)
	}

	hub.System = runtime.NewSystem(ctx, "hub", closers...)

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

		runs, err := hub.store.Runs(hub.Context())

		if err != nil {
			return err
		}

		if runs == nil {
			runs = []tables.Run{}
		}

		return c.JSON(runs)
	})

	hub.app.Use("/hindsight/timeline", func(ctx fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(ctx) {
			ctx.Locals("allowed", true)
			return ctx.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/hindsight/timeline", websocket.New(func(conn *websocket.Conn) {
		if hub.store == nil {
			return
		}

		run := conn.Query("run")

		if run == "" {
			run = conn.Query("epoch")
		}

		if run == "" {
			return
		}

		epoch := parseInt64Query(run)
		symbol := conn.Query("symbol")
		fromTick := parseInt64Query(conn.Query("from"))
		toTick := parseInt64Query(conn.Query("to"))

		const timelineBatchSize = 256
		batch := make([]*data.Measurement[float64], 0, timelineBatchSize)

		for measurement := range hub.store.Timeline(hub.Context(), epoch, symbol, fromTick, toTick) {
			batch = append(batch, measurement)

			if len(batch) < timelineBatchSize {
				continue
			}

			err := types.EncodeMeasurementsFrameWith(batch, func(payload []byte) error {
				return conn.WriteMessage(websocket.BinaryMessage, payload)
			})

			if err != nil {
				return
			}

			batch = batch[:0]
		}

		if len(batch) > 0 {
			err := types.EncodeMeasurementsFrameWith(batch, func(payload []byte) error {
				return conn.WriteMessage(websocket.BinaryMessage, payload)
			})

			if err != nil {
				return
			}
		}
	}, websocket.Config{
		Origins: []string{"*"},
	}))

	hub.app.Get("/hindsight/symbols", func(ctx fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		run := ctx.Query("run")

		if run == "" {
			run = ctx.Query("epoch")
		}

		epoch := parseInt64Query(run)
		symbols, err := hub.store.Symbols(hub.Context(), epoch)

		if err != nil {
			return err
		}

		if symbols == nil {
			symbols = []string{}
		}

		return ctx.JSON(symbols)
	})

	hub.app.Get("/hindsight/excursions", func(ctx fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		run := ctx.Query("run")

		if run == "" {
			run = ctx.Query("epoch")
		}

		epoch := parseInt64Query(run)
		excursions, err := hub.store.Excursions(hub.Context(), epoch, nil)

		if err != nil {
			return err
		}

		if excursions == nil {
			excursions = []tables.ExcursionRecord{}
		}

		return ctx.JSON(excursions)
	})

	hub.app.Get("/hindsight/data", func(ctx fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(ctx.Query("epoch"))
		tableName := ctx.Query("table")

		if tableName == "" {
			tableName = tables.SpotTicker
		}

		limit := int(parseUintQuery(ctx.Query("limit")))
		var measurements []*data.Measurement[float64]

		for measurement := range hub.store.Scan(hub.Context(), tableName, epoch, nil, limit) {
			measurements = append(measurements, measurement)
		}

		if measurements == nil {
			measurements = []*data.Measurement[float64]{}
		}

		return ctx.JSON(measurements)
	})
	hub.registerWorkbench()

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		hub.frontendMu.Lock()
		hub.frontend = conn
		hub.frontendMu.Unlock()
		errnie.Info("hub: frontend websocket connected")

		defer func() {
			hub.frontendMu.Lock()
			if hub.frontend == conn {
				hub.frontend = nil
			}
			hub.frontendMu.Unlock()
			errnie.Info("hub: frontend websocket disconnected")
			conn.Conn.Close()
		}()

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				_, payload, err := conn.Conn.ReadMessage()

				if err != nil {
					return
				}

				hub.handleCommand(payload)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if hub.uiTee == nil {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			payload := hub.uiTee.Next()

			if len(payload) == 0 {
				time.Sleep(100 * time.Microsecond)
				continue
			}

			err := conn.Conn.WriteMessage(websocket.BinaryMessage, payload)

			if err != nil {
				return
			}
		}
	}, websocket.Config{
		Origins: []string{"*"},
	}))

	hub.registerFluidWebRTC()

	return hub
}

/*
SetHindsightStore attaches the capture store so the Hindsight inspection reads
can answer without the live path.
*/
func (hub *Hub) SetHindsightStore(store *tables.Catalog) {
	if hub == nil {
		return
	}

	hub.store = store
}

/*
SetTradeStore attaches the broker's trade journal so GET /trades can serve the
persisted position_trades table.
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
Fluid returns the WebRTC transport boundary used by live visualization viewers.
*/
func (hub *Hub) Fluid() *FluidRTC {
	if hub == nil {
		return nil
	}

	return hub.fluid
}

/*
PhysicsMonitor returns the physics monitor attached to the hub.
*/
func (hub *Hub) PhysicsMonitor() *sensorium.PhysicsMonitor {
	if hub == nil {
		return nil
	}

	return &hub.physics
}

/*
Close shuts down the HTTP server, cancels clients, and waits for ingress drain.
*/
func (hub *Hub) Close() error {
	var err error

	if hub.System != nil {
		err = hub.System.Close()
	}

	if hub.fluid != nil {
		err = errors.Join(err, hub.fluid.Close())
	}

	if hub.app != nil {
		if shutdownErr := hub.app.Shutdown(); shutdownErr != nil {
			if !errors.Is(shutdownErr, net.ErrClosed) && !strings.Contains(shutdownErr.Error(), "use of closed network connection") {
				err = errors.Join(err, shutdownErr)
			}
		}
	}

	return err
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
		Route     string `json:"route"`
		At        string `json:"at"`
		CaptureID int64  `json:"captureId"`
	}

	if err := sonic.Unmarshal(payload, &request); err != nil {
		return
	}

	switch request.Type {
	case "focus":
		types.SetFocus(request.Symbol)
	case "route":
		types.SetRoute(request.Route)
	case "position.exit":
		if hub.exitHandler != nil && request.Symbol != "" {
			hub.exitHandler(request.Symbol)
		}
	}
}

/*
Run listens for dashboard clients on the configured address.
*/
func (hub *Hub) Run() error {
	address := hub.listenAddr

	if address == "" {
		address = ":8765"
	}

	return hub.app.Listen(address)
}
