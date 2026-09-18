package ui

import (
	"context"
	"iter"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/strategy"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/lf"
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
	*runtime.System
	queue      *lf.Queue[unsafe.Pointer]
	app        *fiber.App
	listenAddr string
	store      *tables.Catalog
	tradeStore TradeJournalSource
	trader     *strategy.Trader
	desk       *broker.Desk
	connected  *atomic.Bool
}

/*
NewHub constructs the dashboard hub from its queue-backed system boundaries and
registers it on the workspace so live frames reach it through Step.
*/
func NewHub(
	ctx context.Context,
	trades TradeJournalSource,
	hindsightStore *tables.Catalog,
	trader *strategy.Trader,
	desk *broker.Desk,
) *Hub {
	viper.SetDefault("ui.addr", "127.0.0.1:8765")
	viper.SetDefault("ui.websocket.max_message_bytes", 4*1024*1024)

	hub := &Hub{
		queue:      lf.NewQueue[unsafe.Pointer](),
		listenAddr: viper.GetString("ui.addr"),
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  4194304,
			WriteBufferSize: 4194304,
		}),
		trader:     trader,
		desk:       desk,
		tradeStore: trades,
		store:      hindsightStore,
		connected:  &atomic.Bool{},
	}

	hub.System = runtime.NewSystem(ctx, "hub", hub)

	hub.app.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{"*"},
	}))

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		errnie.Info("hub: frontend websocket connected")
		hub.connected.Store(true)

		defer func() {
			errnie.Info("hub: frontend websocket disconnected")
			hub.connected.Store(false)
			conn.Conn.Close()
		}()

		go func() {
			for {
				select {
				case <-ctx.Done():
					hub.Close()
					return
				default:
				}

				_, payload, err := conn.Conn.ReadMessage()

				if err != nil {
					hub.Error(errnie.Err(
						errnie.BadRequest,
						"[hub] failed to read message from frontend websocket",
						err,
					))

					return
				}

				hub.Command(payload)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				hub.Close()
				return
			default:
			}

			if hub.queue.Length() == 0 {
				time.Sleep(100 * time.Microsecond)
				continue
			}

			frame, ok := hub.queue.Dequeue()

			if !ok || frame == nil {
				time.Sleep(100 * time.Microsecond)
				continue
			}

			if err := conn.Conn.WriteMessage(
				websocket.BinaryMessage, *(*[]byte)(frame),
			); err != nil {
				hub.Error(errnie.Err(
					errnie.BadRequest,
					"[hub] failed to write message to frontend websocket",
					err,
				))

				return
			}
		}
	}, websocket.Config{
		Origins: []string{"*"},
	}))

	return hub
}

/*
Next encodes arriving pipeline data into MeasurementsFrame FlatBuffers and
enqueues the binary payload. Hub is a terminal off-ramp: it yields nothing.
*/
func (hub *Hub) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if hub.Status() != runtime.READY {
			errnie.Warn("pushing to a non ready system may have unintended consequences")
			return
		}

		if in == nil || hub.Error() != nil || !hub.connected.Load() {
			return
		}

		for stream := range in {
			if stream == nil {
				continue
			}

			hub.queue.Enqueue(stream)
		}
	}
}

/*
Command dispatches one inbound JSON command from the dashboard socket.
*/
func (hub *Hub) Command(payload []byte) {
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
		hub.desk.Exit(
			hub.trader.Position(
				request.Symbol,
			),
		)
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
