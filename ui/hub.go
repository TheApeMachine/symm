package ui

import (
	"context"
	"errors"
	"io"
	"syscall"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
Hub owns the dashboard websocket and forwards flat JSON frames to clients.
Each client has a bounded latest-by-key writer queue managed by the hub loop.
Publish coalesces replaceable state by key, assigns a generation, and fans out
only to registered clients so one slow peer cannot block the drain.
*/
type Hub struct {
	ctx        context.Context
	cancel     context.CancelFunc
	app        *fiber.App
	listenAddr string
	Messages   chan []byte
	Manifold   chan []byte
	desk       *broker.Desk
	price      *broker.Price
	balance    *broker.Balance
}

/*
NewHub constructs the dashboard hub from an injected UI config address.
*/
func NewHub(
	ctx context.Context,
	desk *broker.Desk,
	price *broker.Price,
	balance *broker.Balance,
	channel chan []byte,
	manifold chan []byte,
) *Hub {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		ctx:        ctx,
		cancel:     cancel,
		listenAddr: "127.0.0.1:8765",
		Messages:   channel,
		Manifold:   manifold,
		desk:       desk,
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  4 * 1024 * 1024,
			WriteBufferSize: 4 * 1024 * 1024,
		}),
		price:   price,
		balance: balance,
	}

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
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

		/*
			The dashboard tells the engine which symbol it is looking at, and
			focus-gated publishers — resonance above all — only emit for that one.
			Nothing read the socket, so the focus message was discarded and the
			gate stayed on whatever the process started with: a terminal pointed
			at one symbol kept receiving another's latent state, or none at all.

			The reader owns its own goroutine because the write loop below blocks
			on the broadcast channel, and a connection that is only ever written to
			also never observes the client hanging up.
		*/
		go func() {
			for {
				messageType, payload, err := conn.Conn.ReadMessage()

				if err != nil {
					return
				}

				if messageType != websocket.TextMessage {
					continue
				}

				var request struct {
					Type   string `json:"type"`
					Symbol string `json:"symbol"`
				}

				if err := sonic.Unmarshal(payload, &request); err != nil {
					continue
				}

				if request.Type == "focus" {
					types.SetFocus(request.Symbol)
				}
			}
		}()

		for {
			select {
			case <-hub.ctx.Done():
				return
			case msg := <-hub.Messages:
				if err := conn.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					for _, closeError := range []error{
						syscall.EPIPE,
						syscall.ECONNRESET,
						io.EOF,
						io.ErrClosedPipe,
					} {
						if errors.Is(err, closeError) {
							return
						}
					}

					errnie.Error(errnie.Err(
						errnie.IO,
						"failed to write dashboard websocket message",
						err,
					))
				}
			}
		}
	}))

	hub.app.Use("/ws-manifold", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/ws-manifold", websocket.New(func(conn *websocket.Conn) {
		for {
			select {
			case <-hub.ctx.Done():
				return
			case msg := <-hub.Manifold:
				if err := conn.Conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
					for _, closeError := range []error{
						syscall.EPIPE,
						syscall.ECONNRESET,
						io.EOF,
						io.ErrClosedPipe,
					} {
						if errors.Is(err, closeError) {
							return
						}
					}

					errnie.Error(errnie.Err(
						errnie.IO,
						"failed to write dashboard websocket message",
						err,
					))
				}
			}
		}
	}))

	return hub
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

	if hub.app != nil {
		err = hub.app.Shutdown()
	}

	hub.cancel()
	return err
}
