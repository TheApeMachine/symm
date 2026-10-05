package ui

import (
	"context"
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
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
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
PositionSource supplies active open positions and recent decisions for streaming to the UI.
*/
type PositionSource interface {
	PositionsWire() *wire.PositionsFrameT
	DecisionsWire() *wire.StrategyFrameT
	PositionsVersion() uint64
	DecisionsVersion() uint64
}

/*
EquitySource supplies the venue-reported cash, unrealized PnL, and equity.
*/
type EquitySource interface {
	EquityWire() *wire.EquityFrameT
}

type CognitionSource interface {
	CognitionTree() cognition.CognitionTreeExport
}

type FragmentsSource interface {
	Fragments() []TrainedFragment
}

type TrainedFragment struct {
	ID         int             `json:"id"`
	Symbol     string          `json:"symbol"`
	Epoch      int64           `json:"epoch"`
	MarkA      int64           `json:"mark_a"`
	MarkB      int64           `json:"mark_b"`
	MarkC      int64           `json:"mark_c"`
	EntryPrice float64         `json:"entry_price"`
	ExitPrice  float64         `json:"exit_price"`
	Magnitude  float64         `json:"magnitude"`
	Direction  string          `json:"direction"`
	Tokens     []string        `json:"tokens"`
	Points     []FragmentPoint `json:"points"`
	EntryIdx   int             `json:"entry_idx"`
	ExitIdx    int             `json:"exit_idx"`
	LearnedAt  time.Time       `json:"learned_at"`
}

type FragmentPoint struct {
	X    int     `json:"x"`
	Y    float64 `json:"y"`
	Seq  int64   `json:"seq"`
	Time int64   `json:"time"`
}

type Hub struct {
	*runtime.System
	uiTee               *UITee
	workspace           *runtime.Workspace
	physics             sensorium.PhysicsMonitor
	app                 *fiber.App
	listenAddr          string
	frontend            atomic.Pointer[websocket.Conn]
	store               *tables.Catalog
	positionSource      PositionSource
	equitySource        EquitySource
	cognitionSource     CognitionSource
	fragmentsSource     FragmentsSource
	exitHandler         func(symbol string)
	routes              *Routes
	learningInterval    time.Duration
	lastLearning        time.Time
	workbenchSupervisor *WorkbenchSupervisor
}

/*
NewHub constructs the dashboard hub from its queue-backed system boundaries.
The workspace supplies live ingress progress, streamed as TickFrames at display
cadence independently of the route filters applied to the tee.
*/
func NewHub(
	ctx context.Context,
	hindsightStore *tables.Catalog,
	uiTee *UITee,
	workspace *runtime.Workspace,
) *Hub {
	viper.SetDefault("ui.addr", "127.0.0.1:8765")
	viper.SetDefault("ui.websocket.max_message_bytes", 4*1024*1024)

	workbenchURL := viper.GetString("workbench.url")

	if workbenchURL == "" {
		workbenchURL = "http://127.0.0.1:8081/workbench/query"
	}

	workbenchSupervisor := NewWorkbenchSupervisor(workbenchURL)

	hub := &Hub{
		learningInterval:    viper.GetDuration("ui.websocket.learning_interval"),
		uiTee:               uiTee,
		workspace:           workspace,
		listenAddr:          viper.GetString("ui.addr"),
		workbenchSupervisor: workbenchSupervisor,
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

	closers := []io.Closer{uiTee}

	if workbenchSupervisor != nil {
		closers = append(closers, workbenchSupervisor)
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
	hub.registerWorkbench()

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		hub.frontend.Store(conn)
		errnie.Info("hub: frontend websocket connected")

		defer func() {
			hub.frontend.CompareAndSwap(conn, nil)
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
			if hub.positionSource == nil {
				return nil
			}

			ver := hub.positionSource.DecisionsVersion()
			if ver != 0 && ver == lastDecisionsVersion {
				return nil
			}

			wireFrame := hub.positionSource.DecisionsWire()

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

		frameTicker := time.NewTicker(16666 * time.Microsecond)
		defer frameTicker.Stop()

		teeAvailable := hub.uiTee.Available()

		for {
			select {
			case <-ctx.Done():
				return
			case <-teeAvailable:
			case <-frameTicker.C:
				if err := sendTick(); err != nil {
					return
				}
			}

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

			if hub.Status() != runtime.READY {
				continue
			}

			for {
				frame := hub.uiTee.Next()

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
	}))

	return hub
}

/*
SetPositionSource attaches the source for active open positions and decisions.
*/
func (hub *Hub) SetPositionSource(source PositionSource) {
	if hub == nil {
		return
	}

	hub.positionSource = source
}

/*
SetEquitySource attaches the venue account state shown in the top bar.
*/
func (hub *Hub) SetEquitySource(source EquitySource) {
	if hub == nil {
		return
	}

	hub.equitySource = source
}


/*
SetCognitionSource attaches the source for active cognitive memory and trie topology.
*/
func (hub *Hub) SetCognitionSource(source CognitionSource) {
	if hub == nil {
		return
	}

	hub.cognitionSource = source
}

/*
SetFragmentsSource attaches the source for already trained fragments.
*/
func (hub *Hub) SetFragmentsSource(source FragmentsSource) {
	if hub == nil {
		return
	}

	hub.fragmentsSource = source
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
	}

	return b.String()
}
