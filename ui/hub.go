package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/bytedance/sonic"
	fastwebsocket "github.com/fasthttp/websocket"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
consumerSeq numbers each websocket client's private consumer cursor so clients
drain the shared lock-free transport independently without sharing a cursor.
*/
var consumerSeq atomic.Uint64

func consumerIDFor() string {
	return "dashboard-" + fmt.Sprint(consumerSeq.Add(1))
}

/*
Hub owns the dashboard websocket and broadcasts flat JSON frames to clients.
Each client has a bounded writer queue; a peer that cannot keep up is closed
with an observable error so it cannot block market telemetry for every peer.
*/
type Hub struct {
	ctx        context.Context
	cancel     context.CancelFunc
	app        *fiber.App
	listenAddr string
	thesis     *types.Thesis
	desk       *broker.Desk
	price      *broker.Price
	balance    *broker.Balance
	playback   playback
	captures   func() any
	fluid      *FluidRTC
	manifold   *transport.MapReduce[types.FluidFrame]
}

/*
NewHub constructs the dashboard hub from an injected UI config address.
*/
type playback interface {
	Play()
	Pause()
	Seek(at time.Time)
	Select(captureID int64)
	Hindsight(captureID int64)
}

func NewHub(
	ctx context.Context,
	thesis *types.Thesis,
	desk *broker.Desk,
	price *broker.Price,
	balance *broker.Balance,
	manifold *transport.MapReduce[types.FluidFrame],
) *Hub {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		ctx:        ctx,
		cancel:     cancel,
		listenAddr: "127.0.0.1:8765",
		thesis:     thesis,
		desk:       desk,
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  4 * 1024 * 1024,
			WriteBufferSize: 4 * 1024 * 1024,
		}),
		price:     price,
		balance:   balance,
		fluid:     NewFluidRTC(ctx),
		manifold:  manifold,
	}

	if manifold != nil {
		go hub.fluid.Run(manifold, "fluid")
	}

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/backtest/captures", func(c fiber.Ctx) error {
		if hub.captures == nil {
			return c.JSON([]any{})
		}

		return c.JSON(hub.captures())
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		var ui *transport.MapReduce[[]byte]

		if hub.thesis != nil {
			ui = hub.thesis.UI()
		}

		consumerID := consumerIDFor()

		// Each client drains the lock-free UI transport under its own consumer
		// cursor. MapReduce fans every pushed frame out to every registered
		// consumer, so each client receives each frame without any shared
		// broadcast or client-fan-out machinery on the hub.
		if ui != nil {
			ui.Register(consumerID)
		}

		if hub.balance != nil {
			conn.WriteMessage(websocket.TextMessage, hub.balance.Wallet())
		}

		if hub.desk != nil {
			out := make([]*broker.Position, 0)

			for position := range hub.desk.Positions() {
				out = append(out, position)
			}

			conn.WriteMessage(websocket.TextMessage, datura.NewMap(
				"positions", out,
			).MarshalAndFree())
		}

		// The capture list rides the websocket with the rest of the
		// dashboard state: the dev-server origin cannot fetch the REST
		// route cross-origin, and the socket already reaches every client.
		if hub.captures != nil {
			conn.WriteMessage(websocket.TextMessage, datura.NewMap(
				"backtest", datura.NewMap("captures", hub.captures()),
			).MarshalAndFree())
		}

		clientDone := make(chan struct{})

		go func() {
			defer close(clientDone)

			for {
				select {
				case <-hub.ctx.Done():
					return
				default:
					messageType, payload, err := conn.Conn.ReadMessage()

					if err != nil {
						return
					}

					if messageType != websocket.TextMessage {
						continue
					}

					var request struct {
						Type      string `json:"type"`
						Symbol    string `json:"symbol"`
						At        string `json:"at"`
						CaptureID int64  `json:"captureId"`
					}

					if err := sonic.Unmarshal(payload, &request); err != nil {
						continue
					}

					switch request.Type {
					case "focus":
						types.SetFocus(request.Symbol)
					case "backtest.play":
						if hub.playback != nil {
							hub.playback.Play()
						}
					case "backtest.pause":
						if hub.playback != nil {
							hub.playback.Pause()
						}
					case "backtest.seek":
						if hub.playback != nil {
							if at, err := time.Parse(time.RFC3339Nano, request.At); err == nil {
								hub.playback.Seek(at)
							}
						}
					case "backtest.select":
						if hub.playback != nil {
							hub.playback.Select(request.CaptureID)
						}
					case "backtest.hindsight":
						if hub.playback != nil {
							hub.playback.Hindsight(request.CaptureID)
						}
					}
				}
			}
		}()

		defer func() {
			_ = conn.Close()
			<-clientDone
		}()

		for {
			select {
			case <-hub.ctx.Done():
				return
			case <-clientDone:
				return
			default:
				frame, ok := ui.Pop(consumerID)

				if !ok {
					runtime.Gosched()
					continue
				}

				if err := conn.Conn.WriteMessage(websocket.TextMessage, frame); err != nil {
					if !expectedDashboardWriteClosure(err) {
						errnie.Error(errnie.Err(
							errnie.UnprocessableContent,
							"dashboard: write failed: "+err.Error(),
							err,
						))
					}

					return
				}
			}
		}
	}))

	hub.registerFluidWebRTC()

	return hub
}

/*
SetPlayback attaches the backtest driver and its capture listing so websocket
commands and the REST route reach it. Without a driver the controls are inert.
*/
func (hub *Hub) SetPlayback(
	controller interface {
		Play()
		Pause()
		Seek(at time.Time)
		Select(captureID int64)
		Hindsight(captureID int64)
	},
	captures func() any,
) {
	// Upgrade-only: a session re-boot passes a nil controller with a fresh
	// capture list and must never displace the driving playback controller.
	if controller != nil {
		hub.playback = controller
	}

	if captures != nil {
		hub.captures = captures
	}
}

/*
expectedDashboardWriteClosure identifies transport errors that only report an
already completed dashboard disconnect.
*/
func expectedDashboardWriteClosure(err error) bool {
	for _, expected := range []error{
		syscall.EPIPE,
		syscall.ECONNRESET,
		io.EOF,
		io.ErrClosedPipe,
		fastwebsocket.ErrCloseSent,
	} {
		if errors.Is(err, expected) {
			return true
		}
	}

	return false
}

/*
Serve listens for dashboard websocket clients.
*/
func (hub *Hub) Serve() error {
	return hub.app.Listen(hub.listenAddr)
}

/*
Close shuts down the HTTP server, cancels clients, and waits for ingress drain.
*/
func (hub *Hub) Close() error {
	var err error

	hub.cancel()
	err = errors.Join(err, hub.fluid.Close())

	if hub.app != nil {
		err = hub.app.Shutdown()
	}

	return err
}
