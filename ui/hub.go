package ui

import (
	"context"
	"fmt"
	"io"
	neturl "net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/nomagique/runtime"
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
PositionSource supplies active open positions for streaming to the UI.
*/
type PositionSource interface {
	PositionsWire() *wire.PositionsFrameT
	PositionsVersion() uint64
}

/*
DecisionSource supplies the latest strategy decisions for streaming to the UI.
*/
type DecisionSource interface {
	DecisionsWire() *wire.StrategyFrameT
	DecisionsVersion() uint64
}

/*
EquitySource supplies the venue-reported cash, unrealized PnL, and equity.
*/
type EquitySource interface {
	EquityWire() *wire.EquityFrameT
}

type CognitionSource interface {
	CognitionTree() CognitionTreeExport
}

type FragmentsSource interface {
	Fragments() []TrainedFragment
	FragmentPoints(id int) ([]FragmentPoint, error)
}

/*
LifecycleSource supplies the per-position event timelines of the paper desk
and the performance of the closed positions, as a JSON-ready report.
*/
type LifecycleSource interface {
	LifecyclesReport() any
}

type LearningSource interface {
	LearningSummary() string
	LearningReport() any
}

type TrainedFragment struct {
	ID                int             `json:"id"`
	Symbol            string          `json:"symbol"`
	Epoch             int64           `json:"epoch"`
	MarkA             int64           `json:"mark_a"`
	MarkB             int64           `json:"mark_b"`
	MarkC             int64           `json:"mark_c"`
	EntryPrice        float64         `json:"entry_price"`
	ExitPrice         float64         `json:"exit_price"`
	Magnitude         float64         `json:"magnitude"`
	Direction         string          `json:"direction"`
	Class             string          `json:"class"`
	Tokens            []string        `json:"tokens"`
	Points            []FragmentPoint `json:"points"`
	EntryIdx          int             `json:"entry_idx"`
	ExitIdx           int             `json:"exit_idx"`
	PredictedEntryIdx int             `json:"predicted_entry_idx"`
	PredictedExitIdx  int             `json:"predicted_exit_idx"`
	LearnedAt         time.Time       `json:"learned_at"`
}

/*
FragmentPoint is one chart point of a trained fragment. Tick is the trade's
market tick (not its sequence index); the wire name stays "seq" for the
frontend contract.
*/
type FragmentPoint struct {
	X    int     `json:"x"`
	Y    float64 `json:"y"`
	Tick int64   `json:"seq"`
	Time int64   `json:"time"`
}

type Hub struct {
	*runtime.System
	uiTees           []*UITee
	storeTee         *hindsight.StoreTee
	workspace        *runtime.Workspace
	physics          sensorium.PhysicsMonitor
	app              *fiber.App
	listenAddr       string
	frontend         atomic.Pointer[websocket.Conn]
	store            *tables.Catalog
	positionSource   PositionSource
	decisionSource   DecisionSource
	equitySource     EquitySource
	cognitionSource  CognitionSource
	fragmentsSource  FragmentsSource
	learningSource   LearningSource
	lifecycleSource  LifecycleSource
	exitHandler      func(symbol string)
	routes           *Routes
	learningInterval time.Duration
	lastLearning     time.Time
}

func (hub *Hub) teeForShard(shard int) *UITee {
	if shard >= 0 && shard < len(hub.uiTees) {
		return hub.uiTees[shard]
	}

	if len(hub.uiTees) > 0 {
		return hub.uiTees[0]
	}

	return nil
}

/*
NewHub constructs the dashboard hub from its queue-backed system boundaries.
The workspace supplies live ingress progress, streamed as TickFrames at display
cadence independently of the route filters applied to the tee.
*/
func NewHub(
	ctx context.Context,
	hindsightStore *tables.Catalog,
	workspace *runtime.Workspace,
	storeTee *hindsight.StoreTee,
	equitySource EquitySource,
	positionSource PositionSource,
	decisionSource DecisionSource,
	cognitionSource CognitionSource,
	fragmentsSource FragmentsSource,
	lifecycleSource LifecycleSource,
	exitHandler func(symbol string),
	uiTees ...*UITee,
) *Hub {
	viper.SetDefault("ui.addr", "127.0.0.1:8765")
	viper.SetDefault("ui.websocket.max_message_bytes", 4*1024*1024)

	workbenchURL := viper.GetString("workbench.url")

	if workbenchURL == "" {
		workbenchURL = "http://127.0.0.1:8081/workbench/query"
	}

	hub := &Hub{
		learningInterval: viper.GetDuration("ui.websocket.learning_interval"),
		uiTees:           uiTees,
		workspace:        workspace,
		storeTee:         storeTee,
		equitySource:     equitySource,
		positionSource:   positionSource,
		decisionSource:   decisionSource,
		cognitionSource:  cognitionSource,
		fragmentsSource:  fragmentsSource,
		lifecycleSource:  lifecycleSource,
		exitHandler:      exitHandler,
		listenAddr:       viper.GetString("ui.addr"),
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  4194304,
			WriteBufferSize: 4194304,
		}),
		store: hindsightStore,
	}

	hub.routes = NewRoutes(hub)

	closers := make([]io.Closer, 0, len(uiTees))
	for _, tee := range uiTees {
		closers = append(closers, tee)
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

	hub.routes.Register()

	wsHandler := func(shard int) fiber.Handler {
		return websocket.New(func(conn *websocket.Conn) {
			if shard == 0 {
				hub.frontend.Store(conn)
			}
			errnie.Info(fmt.Sprintf("[hub] frontend websocket shard %d connected", shard))

			defer func() {
				if shard == 0 {
					hub.frontend.CompareAndSwap(conn, nil)
				}
				errnie.Info(fmt.Sprintf("[hub] frontend websocket shard %d disconnected", shard))
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

			var lastPositionsPush time.Time
			var lastPositionsVersion uint64
			var lastPositionsSummary string
			hadPositions := false

			sendPositions := func() error {
				if hub.positionSource == nil {
					return nil
				}

				ver := hub.positionSource.PositionsVersion()
				wireFrame := hub.positionSource.PositionsWire()

				if wireFrame == nil {
					return nil
				}

				hasNow := len(wireFrame.Rows) > 0

				if !hasNow && !hadPositions {
					return nil
				}

				summary := positionsSummary(wireFrame.Rows)

				if ver != 0 && ver == lastPositionsVersion && summary == lastPositionsSummary {
					return nil
				}

				hadPositions = hasNow
				lastPositionsVersion = ver
				lastPositionsSummary = summary

				message := &wire.MessageT{
					Sequence: uint64(time.Now().UnixNano()),
					Frame: &wire.FrameT{
						Type:  wire.FramePositionsFrame,
						Value: wireFrame,
					},
				}

				builder := flatbuffers.NewBuilder(4096)
				offset := message.Pack(builder)
				builder.FinishWithFileIdentifier(offset, []byte("SYMM"))
				payload := builder.FinishedBytes()

				lastPositionsPush = time.Now()
				return conn.Conn.WriteMessage(websocket.BinaryMessage, payload)
			}

			var lastDecisionsPush time.Time
			var lastDecisionsVersion uint64

			sendDecisions := func() error {
				if hub.decisionSource == nil {
					return nil
				}

				ver := hub.decisionSource.DecisionsVersion()
				if ver != 0 && ver == lastDecisionsVersion {
					return nil
				}

				wireFrame := hub.decisionSource.DecisionsWire()

				if wireFrame == nil || len(wireFrame.Decisions) == 0 {
					return nil
				}

				lastDecisionsVersion = ver

				message := &wire.MessageT{
					Sequence: uint64(time.Now().UnixNano()),
					Frame: &wire.FrameT{
						Type:  wire.FrameStrategyFrame,
						Value: wireFrame,
					},
				}

				builder := flatbuffers.NewBuilder(4096)
				offset := message.Pack(builder)
				builder.FinishWithFileIdentifier(offset, []byte("SYMM"))
				payload := builder.FinishedBytes()

				lastDecisionsPush = time.Now()
				return conn.Conn.WriteMessage(websocket.BinaryMessage, payload)
			}

			var lastEquityPush time.Time
			var lastCash, lastUnrealized, lastEquity string

			sendEquity := func() error {
				if hub.equitySource == nil {
					return nil
				}

				wireFrame := hub.equitySource.EquityWire()

				if wireFrame == nil {
					return nil
				}

				if wireFrame.Cash == lastCash && wireFrame.Unrealized == lastUnrealized && wireFrame.Equity == lastEquity {
					return nil
				}

				lastCash = wireFrame.Cash
				lastUnrealized = wireFrame.Unrealized
				lastEquity = wireFrame.Equity

				message := &wire.MessageT{
					Sequence: uint64(time.Now().UnixNano()),
					Frame: &wire.FrameT{
						Type:  wire.FrameEquityFrame,
						Value: wireFrame,
					},
				}

				builder := flatbuffers.NewBuilder(1024)
				offset := message.Pack(builder)
				builder.FinishWithFileIdentifier(offset, []byte("SYMM"))
				payload := builder.FinishedBytes()

				lastEquityPush = time.Now()
				return conn.Conn.WriteMessage(websocket.BinaryMessage, payload)
			}

			var lastTick int64

			sendTick := func() error {
				tick, at := hub.workspace.Progress()

				if tick == lastTick {
					return nil
				}

				lastTick = tick

				message := &wire.MessageT{
					Sequence: uint64(time.Now().UnixNano()),
					Frame: &wire.FrameT{
						Type:  wire.FrameTickFrame,
						Value: &wire.TickFrameT{Count: tick, At: at},
					},
				}

				builder := flatbuffers.NewBuilder(64)
				offset := message.Pack(builder)
				builder.FinishWithFileIdentifier(offset, []byte("SYMM"))

				return conn.Conn.WriteMessage(websocket.BinaryMessage, builder.FinishedBytes())
			}

			var lastDiagnosticsPush time.Time

			sendDiagnostics := func() error {
				now := time.Now()
				nowNs := now.UnixNano()
				var rows []*wire.MeasurementT

				var uiPending, uiIngress, uiEgress int
				for _, tee := range hub.uiTees {
					if tee != nil {
						uiPending += tee.Pending()
						uiIngress += tee.IngressLength()
						uiEgress += tee.EgressLength()
					}
				}

				rows = append(rows, &wire.MeasurementT{
					Source: "ui_tee",
					At:     nowNs,
					Metrics: []*wire.MetricT{
						{Name: "backlog", Raw: float64(uiPending)},
						{Name: "ingress", Raw: float64(uiIngress)},
						{Name: "egress", Raw: float64(uiEgress)},
					},
					Metadata: []*wire.NamedNumberT{
						{Name: "stage", Value: 4},
					},
				})

				if hub.storeTee != nil {
					storePending := hub.storeTee.Pending()
					rows = append(rows, &wire.MeasurementT{
						Source: "store_tee",
						At:     nowNs,
						Metrics: []*wire.MetricT{
							{Name: "backlog", Raw: float64(storePending)},
						},
						Metadata: []*wire.NamedNumberT{
							{Name: "stage", Value: 4},
						},
					})
				}

				if len(rows) == 0 {
					return nil
				}

				payload := types.PackMeasurementsFrame(rows)

				if len(payload) == 0 {
					return nil
				}

				lastDiagnosticsPush = now
				return conn.Conn.WriteMessage(websocket.BinaryMessage, payload)
			}

			if shard == 0 {
				if err := sendPositions(); err != nil {
					return
				}

				if err := sendDecisions(); err != nil {
					return
				}

				if err := sendEquity(); err != nil {
					return
				}

				if err := sendTick(); err != nil {
					return
				}

				if err := sendDiagnostics(); err != nil {
					return
				}
			}

			tee := hub.teeForShard(shard)
			frameTicker := time.NewTicker(16666 * time.Microsecond)
			defer frameTicker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-frameTicker.C:
					if shard == 0 {
						if err := sendTick(); err != nil {
							return
						}
					}
				}

				if shard == 0 {
					if time.Since(lastPositionsPush) >= 200*time.Millisecond {
						if err := sendPositions(); err != nil {
							return
						}
					}

					if time.Since(lastDecisionsPush) >= 1000*time.Millisecond {
						if err := sendDecisions(); err != nil {
							return
						}
					}

					if time.Since(lastEquityPush) >= 500*time.Millisecond {
						if err := sendEquity(); err != nil {
							return
						}
					}

					if time.Since(lastDiagnosticsPush) >= 100*time.Millisecond {
						if err := sendDiagnostics(); err != nil {
							return
						}
					}
				}

				if hub.Status() != runtime.READY || tee == nil {
					continue
				}

				for {
					frame := tee.Next()

					if frame == nil {
						break
					}

					payload := *(*[]byte)(frame)

					if len(payload) == 0 {
						continue
					}

					if err := conn.Conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
						return
					}
				}
			}
		}, websocket.Config{
			Origins: []string{"*"},
		})
	}

	hub.app.Get("/ws", wsHandler(0))
	hub.app.Get("/ws/0", wsHandler(0))
	hub.app.Get("/ws/1", wsHandler(1))

	return hub
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
		if hub.exitHandler != nil {
			hub.exitHandler(request.Symbol)
		}
	}
}

/*
Run listens for dashboard clients on the configured address.
*/
func (hub *Hub) Run() {
	go func() {
		address := hub.listenAddr

		if address == "" {
			address = ":8765"
		}

		hub.app.Listen(address)
	}()
}

func positionsSummary(rows []*wire.PositionT) string {
	if len(rows) == 0 {
		return ""
	}

	var b strings.Builder

	for _, row := range rows {
		if row == nil || row.Holding == nil {
			continue
		}

		b.WriteString(row.Holding.Symbol)
		b.WriteString(row.Holding.Status)
		b.WriteString(row.Holding.Qty)
		b.WriteString(row.Holding.Mark)
		b.WriteString(row.Holding.Pnl)
		b.WriteString(row.Holding.VenuePnl)
		b.WriteString(row.Holding.ShadowPnl)
	}

	return b.String()
}
