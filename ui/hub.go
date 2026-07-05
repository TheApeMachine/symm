package ui

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

/*
Hub owns the dashboard websocket and forwards typed backend frames to clients.
*/
type Hub struct {
	ctx            context.Context
	cancel         context.CancelFunc
	app            *fiber.App
	listenAddr     string
	messages       chan Message
	clients        sync.Map
	clientSequence atomic.Uint64
	snapshot       *Snapshot
}

func NewHub(ctx context.Context) (*Hub, error) {
	ctx, cancel := context.WithCancel(ctx)
	listenAddr := viper.GetString("ui.addr")

	if listenAddr == "" {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"ui: listen address is required (ui.addr)",
			nil,
		))
	}

	buffer := viper.GetInt("system.websocket.channel.buffer")
	if buffer <= 0 {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"ui: websocket frame buffer is required (system.websocket.channel.buffer)",
			nil,
		))
	}

	hub := &Hub{
		ctx:        ctx,
		cancel:     cancel,
		listenAddr: listenAddr,
		messages:   make(chan Message, buffer),
		snapshot:   NewSnapshot(),
		app: fiber.New(fiber.Config{
			JSONEncoder:   sonic.Marshal,
			JSONDecoder:   sonic.Unmarshal,
			StrictRouting: true,
		}),
	}

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		clientID := fmt.Sprintf("ui/%d", hub.clientSequence.Add(1))

		defer func() {
			errnie.Error(conn.Close())
		}()

		if errnie.Error(hub.snapshot.Replay(conn)) != nil {
			return
		}

		hub.clients.Store(clientID, conn)
		defer hub.clients.Delete(clientID)

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))

	go hub.relay()

	return hub, nil
}

func (hub *Hub) Publish(message Message) error {
	if message.Empty() {
		return errnie.Error(errnie.Err(errnie.Validation, "ui: empty message", nil))
	}

	select {
	case <-hub.ctx.Done():
		return errnie.Error(errnie.Err(errnie.Canceled, "ui: hub closed", hub.ctx.Err()))
	case hub.messages <- message:
		return nil
	}
}

func (hub *Hub) relay() {
	for {
		select {
		case <-hub.ctx.Done():
			return
		case message := <-hub.messages:
			if err := hub.snapshot.Observe(message); errnie.Error(err) != nil {
				return
			}

			hub.clients.Range(func(key, value any) bool {
				conn := value.(*websocket.Conn)
				if err := writeMessage(conn, message); err != nil {
					errnie.Error(errnie.Err(errnie.IO, "ui: websocket write failed", err))
					hub.clients.Delete(key)
					errnie.Error(conn.Close())
				}

				return true
			})
		}
	}
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

func writeMessage(conn *websocket.Conn, message Message) error {
	wire, err := sonic.Marshal(message)
	if err != nil {
		return errnie.Err(errnie.Validation, "ui: encode message", err)
	}

	return conn.WriteMessage(websocket.TextMessage, wire)
}
