package ui

import (
	"bytes"
	"context"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
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
	cache       *sync.Map
	subscribers *sync.Map
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
		Messages:   make(chan []byte, buffer),
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  8 * 1024,
			WriteBufferSize: 8 * 1024,
		}),
		cache:       &sync.Map{},
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

		hub.subscribers.Store(conn.Conn.RemoteAddr().String(), conn)
		defer hub.subscribers.Delete(conn.Conn.RemoteAddr().String())

		for _, key := range cacheKeys {
			found, ok := hub.cache.Load(key)

			if ok {
				errnie.Error(conn.Conn.WriteMessage(
					websocket.TextMessage, found.([]byte),
				))
			}
		}

		for {
			select {
			case <-hub.ctx.Done():
				return
			case msg := <-hub.Messages:
				if conn.Conn.WriteMessage(
					websocket.TextMessage, msg,
				) != nil {
					return
				}

				for _, key := range cacheKeys {
					if bytes.Contains(msg, []byte(`"`+key+`"`)) {
						hub.cache.Store(key, msg)
					}
				}
			}
		}
	}))

	return hub, nil
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
