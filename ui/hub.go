package ui

import (
	"context"
	"errors"
	"io"
	"sync"
	"syscall"

	"github.com/bytedance/sonic"
	fastwebsocket "github.com/fasthttp/websocket"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

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
	Messages   chan []byte
	desk       *broker.Desk
	price      *broker.Price
	balance    *broker.Balance
	fluid      *FluidRTC
	clients    sync.Map
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
	manifold chan types.FluidFrame,
) *Hub {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		ctx:        ctx,
		cancel:     cancel,
		listenAddr: "127.0.0.1:8765",
		Messages:   channel,
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
		fluid:   NewFluidRTC(ctx),
	}

	go hub.fluid.Run(manifold)
	go hub.broadcast()

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		queueCapacity := cap(hub.Messages)

		if queueCapacity < 1 {
			queueCapacity = 1
		}

		messages := make(chan []byte, queueCapacity)
		hub.clients.Store(conn, messages)
		defer hub.clients.Delete(conn)

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
			case msg := <-messages:
				if err := conn.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					if expectedDashboardWriteClosure(err) {
						return
					}

					errnie.Error(errnie.Err(
						errnie.IO,
						"failed to write dashboard websocket message: "+err.Error(),
						err,
					))

					return
				}
			}
		}
	}))

	hub.registerFluidWebRTC()

	return hub
}

func (hub *Hub) broadcast() {
	for {
		select {
		case <-hub.ctx.Done():
			return
		case message := <-hub.Messages:
			hub.clients.Range(func(key, value any) bool {
				conn := key.(*websocket.Conn)
				messages := value.(chan []byte)

				select {
				case messages <- message:
				default:
					errnie.Error(errnie.Err(
						errnie.IO,
						"dashboard client queue exhausted",
						nil,
					))
					hub.clients.Delete(conn)
					_ = conn.Close()
				}

				return true
			})
		}
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
