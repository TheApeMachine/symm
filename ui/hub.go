package ui

import (
	"bytes"
	"context"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

var hubReplayRoles = []string{"wallet", "state", "story"}

/*
Hub subscribes to the ui broadcast group and forwards frames to the dashboard
websocket client.
*/
type Hub struct {
	ctx            context.Context
	cancel         context.CancelFunc
	tree           *dmt.Tree
	uiBroadcast    *qpool.BroadcastGroup
	uiSubscription *qpool.BroadcastConsumer
	app            *fiber.App
	listenAddr     string
	clients        sync.Map
	cachedWire     sync.Map
}

func NewHub(
	ctx context.Context,
	pool *qpool.Q[any],
) *Hub {
	ctx, cancel := context.WithCancel(ctx)
	listenAddr := viper.GetString("ui.addr")

	if listenAddr == "" {
		listenAddr = "127.0.0.1:8765"
	}

	hub := &Hub{
		ctx:            ctx,
		cancel:         cancel,
		tree:           dmt.NewTree(""),
		uiBroadcast:    pool.CreateBroadcastGroup("ui"),
		uiSubscription: pool.Subscribe("ui", nil),
		listenAddr:     listenAddr,
		app: fiber.New(fiber.Config{
			JSONEncoder:   sonic.Marshal,
			JSONDecoder:   sonic.Unmarshal,
			StrictRouting: true,
		}),
	}

	hub.startRelay()

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		hub.clients.Store(conn, conn)
		defer hub.clients.Delete(conn)

		hub.replayCached(conn)

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))

	return hub
}

func (hub *Hub) startRelay() {
	go func() {
		for {
			if hub.ctx.Err() != nil {
				return
			}

			message, err := hub.uiSubscription.Wait(hub.ctx)

			if message == nil || err != nil {
				continue
			}

			hub.fanOut(message)
		}
	}()
}

func (hub *Hub) fanOut(message *datura.Artifact) {
	wire, err := hub.wireBytes(message)

	if err != nil || len(wire) == 0 {
		errnie.Error(errnie.Err(
			errnie.IO,
			"hub: relay wire encode failed",
			err,
		))

		return
	}

	if role, roleErr := message.Role(); roleErr == nil && hub.shouldCacheRole(role) {
		hub.cachedWire.Store(role, append([]byte(nil), wire...))
	}

	hub.clients.Range(func(_, value any) bool {
		hub.writeWire(value.(*websocket.Conn), wire)

		return true
	})
}

func (hub *Hub) wireBytes(message *datura.Artifact) ([]byte, error) {
	buffer := bytes.NewBuffer(nil)

	if _, err := transport.Copy(buffer, message); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func (hub *Hub) shouldCacheRole(role string) bool {
	for _, replayRole := range hubReplayRoles {
		if role == replayRole {
			return true
		}
	}

	return false
}

func (hub *Hub) replayCached(conn *websocket.Conn) {
	for _, role := range hubReplayRoles {
		cached, ok := hub.cachedWire.Load(role)

		if !ok {
			continue
		}

		hub.writeWire(conn, cached.([]byte))
	}
}

func (hub *Hub) writeWire(conn *websocket.Conn, wire []byte) {
	if err := conn.WriteMessage(websocket.BinaryMessage, wire); err != nil {
		hub.clients.Delete(conn)

		errnie.Error(errnie.Err(
			errnie.IO,
			"hub: websocket write failed",
			err,
		))
	}
}

func (hub *Hub) Run() error {
	if err := hub.app.Listen(hub.listenAddr, fiber.ListenConfig{
		EnablePrefork: false,
	}); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"hub: failed to listen",
			err,
		))

		return err
	}

	return nil
}

func (hub *Hub) Close() error {
	if hub.app != nil {
		errnie.Error(hub.app.Shutdown())
	}

	hub.cancel()

	return nil
}
