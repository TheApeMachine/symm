package ui

import (
	"context"
	"errors"
	"io"
	"sync"
	"syscall"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
)

var cacheKeys = []string{"balances", "executions", "instruments", "positions", "tick"}

/*
Hub owns the dashboard websocket and forwards typed backend frames to clients.
*/
type Hub struct {
	ctx         context.Context
	cancel      context.CancelFunc
	app         *fiber.App
	listenAddr  string
	Messages    chan []byte
	price       *broker.Price
	subscribers *sync.Map
}

func NewHub(
	ctx context.Context,
	price *broker.Price,
	balance *broker.Balance,
	channel chan []byte,
) *Hub {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		ctx:        ctx,
		cancel:     cancel,
		listenAddr: viper.GetString("ui.addr"),
		Messages:   channel,
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  4 * 1024 * 1024,
			WriteBufferSize: 4 * 1024 * 1024,
		}),
		price:       price,
		subscribers: &sync.Map{},
	}

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		defer conn.Close()

		for {
			select {
			case <-hub.ctx.Done():
				return
			case msg := <-hub.Messages:
				// FOR THE FINAL TIME: THERE IS ONLY 1 CLIENT, THE FRONTEND,
				// SO: NO, THERE IS NO SITUATION OF MULTIPLE CLIENTS COMPETING
				// FOR THE SAME MESSAGE CHANNEL!

				if conn.Conn == nil {
					return
				}

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

					errnie.Error(err)
					return
				}
			}
		}
	}))

	return hub
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
