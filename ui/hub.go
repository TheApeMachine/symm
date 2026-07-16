package ui

import (
	"maps"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"syscall"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
Hub owns the dashboard websocket and forwards typed backend frames to clients.
*/
type Hub struct {
	status     types.Status
	ctx        context.Context
	cancel     context.CancelFunc
	app        *fiber.App
	listenAddr string
	Messages   chan []byte
	price      *broker.Price
	balance    *broker.Balance
	thesis     atomic.Pointer[types.Thesis]
	projection atomic.Value
}

/*
projection is the frontend's current symbol and signal-source viewport.
*/
type projection struct {
	FocusSymbol string           `json:"focusSymbol"`
	Source      types.SourceType `json:"source"`
}

func NewHub(
	ctx context.Context,
	price *broker.Price,
	balance *broker.Balance,
	thesis *types.Thesis,
	channel chan []byte,
) (*Hub, error) {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		status:     types.INITIALIZING,
		ctx:        ctx,
		cancel:     cancel,
		listenAddr: viper.GetString("ui.addr"),
		Messages:   channel,
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  1024 * 1024,
			WriteBufferSize: 1024 * 1024,
		}),
		price:   price,
		balance: balance,
	}
	view := projection{FocusSymbol: "BTC/USD", Source: types.SourceFluid}
	hub.projection.Store(view)
	thesis.SetUIProjection(view.FocusSymbol, view.Source)
	hub.thesis.Store(thesis)

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		defer conn.Close()
		conn.EnableWriteCompression(true)

		closed := make(chan struct{})

		go func() {
			defer close(closed)

			for {
				control := projection{}

				if err := conn.ReadJSON(&control); err != nil {
					return
				}

				hub.SetProjection(control.FocusSymbol, control.Source)
			}
		}()

		hub.Thesis().Publish()

		for {
			select {
			case <-hub.ctx.Done():
				return
			case <-closed:
				return
			case msg := <-hub.Messages:
				// FOR THE FINAL TIME: THERE IS ONLY 1 CLIENT, THE FRONTEND,
				// SO: NO, THERE IS NO SITUATION OF MULTIPLE CLIENTS COMPETING
				// FOR THE SAME MESSAGE CHANNEL!

				if conn.Conn == nil {
					return
				}

				current, err := hub.currentFrame(msg)

				if err != nil {
					errnie.Error(errnie.Err(
						errnie.Internal,
						"failed to coalesce dashboard frames",
						err,
					))
					continue
				}

				if err := conn.Conn.WriteMessage(websocket.TextMessage, current); err != nil {
					if errors.Is(err, syscall.EPIPE) {
						return
					}

					errnie.Error(err)
					return
				}
			}
		}
	}, websocket.Config{
		EnableCompression: true,
	}))

	return hub, nil
}

/*
currentFrame collapses any queued dashboard snapshots into their newest value
per frame key before the websocket write. The channel is transport buffering,
not browser history, so reconnects must not replay stale analysis states.
*/
func (hub *Hub) currentFrame(first []byte) ([]byte, error) {
	frames := make([][]byte, 0, len(hub.Messages)+1)
	frames = append(frames, first)

	for {
		select {
		case frame := <-hub.Messages:
			frames = append(frames, frame)
		default:
			if len(frames) == 1 {
				return first, nil
			}

			current := make(map[string]json.RawMessage)

			for _, frame := range frames {
				incoming := make(map[string]json.RawMessage)

				if err := sonic.Unmarshal(frame, &incoming); err != nil {
					return nil, err
				}

				maps.Copy(current, incoming)
			}

			return sonic.Marshal(current)
		}
	}
}

func (hub *Hub) Initialize() error {
	errnie.Info("initializing UI hub")
	hub.status = types.READY
	return nil
}

func (hub *Hub) Serve() error {
	return errnie.Error(hub.app.Listen(hub.listenAddr))
}

func (hub *Hub) Close() error {
	if hub.app != nil {
		errnie.Error(hub.app.Shutdown())
	}

	hub.cancel()
	return nil
}

func (hub *Hub) SetThesis(thesis *types.Thesis) {
	hub.thesis.Store(thesis)
	view := hub.projection.Load().(projection)
	thesis.SetUIProjection(view.FocusSymbol, view.Source)
}

/*
Thesis atomically returns the active runtime thesis while ticks replace it.
*/
func (hub *Hub) Thesis() *types.Thesis {
	return hub.thesis.Load()
}

/*
SetProjection applies the frontend's current observability scope to both the
active thesis and every subsequent tick.
*/
func (hub *Hub) SetProjection(symbol string, source types.SourceType) {
	if symbol == "" || source == "" {
		return
	}

	view := projection{FocusSymbol: symbol, Source: source}
	hub.projection.Store(view)
	hub.thesis.Load().SetUIProjection(symbol, source)
}
