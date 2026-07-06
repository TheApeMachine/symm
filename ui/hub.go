package ui

import (
	"bytes"
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
	Messages       chan []byte
	clients        sync.Map
	clientSequence atomic.Uint64

	mu             sync.Mutex
	lastBalances   []byte
	lastExecutions []byte
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

		hub.clients.Store(clientID, conn)
		defer func() {
			hub.clients.Delete(clientID)
			errnie.Error(conn.Close())
		}()

		// Replay the latest balance and executions to the newly connected client
		hub.mu.Lock()
		bal := hub.lastBalances
		exec := hub.lastExecutions
		hub.mu.Unlock()

		if len(bal) > 0 {
			_ = conn.Conn.WriteMessage(websocket.TextMessage, bal)
		}
		if len(exec) > 0 {
			_ = conn.Conn.WriteMessage(websocket.TextMessage, exec)
		}

		for {
			msg := <-hub.Messages

			if bytes.Contains(msg, []byte(`"balances"`)) {
				hub.mu.Lock()
				hub.lastBalances = msg
				hub.mu.Unlock()
			}

			if bytes.Contains(msg, []byte(`"executions"`)) {
				hub.mu.Lock()
				hub.lastExecutions = msg
				hub.mu.Unlock()
			}

			hub.clients.Range(func(key, value any) bool {
				c := value.(*websocket.Conn)
				_ = c.Conn.WriteMessage(websocket.TextMessage, msg)
				return true
			})

		}
	}))

	go func() {
		<-ctx.Done()

		if hub.app != nil {
			errnie.Error(hub.app.Shutdown())
		}
	}()

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
