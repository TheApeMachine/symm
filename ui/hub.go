package ui

import (
	"context"
	"errors"
	"sync"
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
	status      types.Status
	ctx         context.Context
	cancel      context.CancelFunc
	app         *fiber.App
	listenAddr  string
	Messages    chan []byte
	price       *broker.Price
	balance     *broker.Balance
	desk        *broker.Desk
	subscribers *sync.Map
}

func NewHub(
	ctx context.Context,
	price *broker.Price,
	balance *broker.Balance,
	desk *broker.Desk,
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
			ReadBufferSize:  4 * 1024 * 1024,
			WriteBufferSize: 4 * 1024 * 1024,
		}),
		price:       price,
		balance:     balance,
		desk:        desk,
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

		hub.price.Publish()
		hub.balance.Publish()
		hub.desk.Publish()

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
					if errors.Is(err, syscall.EPIPE) {
						return
					}

					errnie.Error(err)
					return
				}
			}
		}
	}))

	return hub, nil
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
