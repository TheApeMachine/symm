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
			ReadBufferSize:  1024 * 1024,
			WriteBufferSize: 1024 * 1024,
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

		key := conn.Conn.RemoteAddr().String()

		// Send initial cache dump BEFORE we register the connection to the active broadcast list.
		// This guarantees zero concurrency over WriteMessage during startup.
		for _, cacheKey := range cacheKeys {
			found, ok := hub.cache.Load(cacheKey)
			if ok {
				payload := found.([]byte)
				if len(payload) > 0 {
					errnie.Error(conn.Conn.WriteMessage(
						websocket.TextMessage, payload,
					))
				}
			}
		}

		// Create a dedicated, lock-free transmission channel for this specific socket
		subChan := make(chan []byte, 1024)
		hub.subscribers.Store(key, subChan)

		defer func() {
			hub.subscribers.Delete(key)
			close(subChan)
		}()

		// Spin up a dedicated writer goroutine for this socket.
		// Now WriteMessage is ONLY ever called by this single goroutine.
		// Absolutely no mutexes required anywhere in the system.
		go func() {
			for msg := range subChan {
				if err := conn.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					conn.Close() // Force ReadMessage to unblock and trigger cleanup
					return
				}
			}
		}()

		// Block on read to keep the socket alive and detect client disconnects
		for {
			if _, _, err := conn.Conn.ReadMessage(); err != nil {
				return
			}
		}
	}))

	go hub.dispatch()

	return hub, nil
}

/*
dispatch fans each published message out to every connected subscriber's channel,
which isolates slow TCP I/O from the hyper-fast application engine.
*/
func (hub *Hub) dispatch() {
	for {
		select {
		case <-hub.ctx.Done():
			return
		case msg := <-hub.Messages:
			hub.subscribers.Range(func(key, value any) bool {
				subChan, ok := value.(chan []byte)

				if ok {
					// Non-blocking fan-out: if the client's network is lagging behind the
					// engine, drop the frame instead of locking up the entire exchange.
					select {
					case subChan <- msg:
					default:
					}
				}

				return true
			})

			for _, cacheKey := range cacheKeys {
				if bytes.Contains(msg, []byte(`"`+cacheKey+`"`)) {
					hub.cache.Store(cacheKey, msg)
				}
			}
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
