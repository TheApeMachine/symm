package ui

import (
	"context"
	"io"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

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
	client         atomic.Pointer[websocket.Conn]
	app            *fiber.App
	listenAddr     string
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

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		for {
			message := errnie.Does(func() (*datura.Artifact, error) {
				return hub.uiSubscription.Wait(hub.ctx)
			}).Or(func(err error) {
				errnie.Error(errnie.Err(
					errnie.IO,
					"hub: failed to wait for message",
					err,
				))
			}).Value()

			writer := errnie.Does(func() (io.WriteCloser, error) {
				return conn.NextWriter(websocket.TextMessage)
			}).Or(func(err error) {
				errnie.Error(errnie.Err(
					errnie.IO,
					"hub: failed to get next writer",
					err,
				))
			}).Value()

			writer.Write(message.DecryptPayload())
		}
	}))

	return hub
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
