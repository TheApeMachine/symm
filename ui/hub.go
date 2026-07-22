package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
)

var cacheKeys = []string{
	"balances", "executions", "instruments", "positions", "tick",
	"holdings", "measurements", "decisions", "lifecycle", "findings",
}

/*
Hub owns the dashboard websocket and forwards typed backend frames to clients.
subscribers retains the latest payload per cacheKeys entry so reconnect replays
state; Messages are drained continuously so publishes are not stranded until a
client attaches.
*/
type Hub struct {
	ctx         context.Context
	cancel      context.CancelFunc
	app         *fiber.App
	listenAddr  string
	Messages    chan []byte
	price       *broker.Price
	subscribers *sync.Map
	client      atomic.Pointer[websocket.Conn]
	writeMu     sync.Mutex
}

/*
NewHub starts the dashboard message drain and websocket endpoint over the
shared production channel.
*/
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
		defer hub.client.CompareAndSwap(conn, nil)
		defer conn.Close()

		hub.client.Store(conn)
		hub.replay()

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))

	go hub.drain()

	return hub
}

/*
drain continuously retains reconnect state and forwards each frame to the
currently attached client without making producers wait for a connection.
*/
func (hub *Hub) drain() {
	for {
		select {
		case <-hub.ctx.Done():
			return
		case msg, ok := <-hub.Messages:
			if !ok {
				return
			}

			hub.retain(msg)
			hub.write(msg)
		}
	}
}

/*
retain caches only reconnect-visible top-level frames and rejects unrelated UI
traffic before JSON decoding so analyzer publication does not tax the hot path.
*/
func (hub *Hub) retain(msg []byte) {
	if len(msg) == 0 {
		return
	}

	cacheable := false

	for _, key := range cacheKeys {
		if bytes.Contains(msg, []byte(key)) {
			cacheable = true
			break
		}
	}

	if !cacheable {
		return
	}

	var frame map[string]sonic.NoCopyRawMessage

	if err := sonic.Unmarshal(msg, &frame); err != nil {
		return
	}

	for _, key := range cacheKeys {
		value, ok := frame[key]

		if !ok {
			continue
		}

		payload, err := sonic.Marshal(map[string]sonic.NoCopyRawMessage{key: value})

		if err != nil || len(payload) == 0 {
			continue
		}

		hub.subscribers.Store(key, payload)
	}
}

/*
replay writes the latest retained frame for each cache key to a new client.
*/
func (hub *Hub) replay() {
	for _, key := range cacheKeys {
		value, ok := hub.subscribers.Load(key)

		if !ok {
			continue
		}

		payload, ok := value.([]byte)

		if !ok || len(payload) == 0 {
			continue
		}

		hub.write(payload)
	}
}

/*
write sends one frame to the active websocket and detaches closed clients.
*/
func (hub *Hub) write(msg []byte) {
	conn := hub.client.Load()

	if conn == nil || conn.Conn == nil || len(msg) == 0 {
		return
	}

	hub.writeMu.Lock()
	defer hub.writeMu.Unlock()

	if err := conn.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
		for _, closeError := range []error{
			syscall.EPIPE,
			syscall.ECONNRESET,
			io.EOF,
			io.ErrClosedPipe,
		} {
			if errors.Is(err, closeError) {
				hub.client.CompareAndSwap(conn, nil)
				return
			}
		}

		errnie.Error(err)
		hub.client.CompareAndSwap(conn, nil)
	}
}

/*
Serve listens for dashboard websocket connections on the configured address.
*/
func (hub *Hub) Serve() error {
	return errnie.Error(hub.app.Listen(hub.listenAddr))
}

/*
Close stops the websocket server and releases the background message drain.
*/
func (hub *Hub) Close() error {
	if hub.app != nil {
		errnie.Error(hub.app.Shutdown())
	}

	hub.cancel()
	return nil
}
