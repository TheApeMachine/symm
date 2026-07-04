package ui

import (
	"context"
	"fmt"
	"sync"

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
	ctx               context.Context
	cancel            context.CancelFunc
	pool              *qpool.Q[any]
	tree              *dmt.Tree
	uiBroadcast       *qpool.BroadcastGroup
	balancesBroadcast *qpool.BroadcastGroup
	app               *fiber.App
	listenAddr        string
	clients           sync.Map
}

func NewHub(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) (*Hub, error) {
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

	hub := &Hub{
		ctx:               ctx,
		cancel:            cancel,
		pool:              pool,
		tree:              tree,
		uiBroadcast:       pool.CreateBroadcastGroup("ui"),
		balancesBroadcast: pool.CreateBroadcastGroup("kraken:private"),
		listenAddr:        listenAddr,
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
		subscriberID := fmt.Sprintf("ui/%p", conn)
		subscription := hub.uiBroadcast.Acquire(subscriberID, nil)

		if subscription == nil {
			return
		}

		defer func() {
			errnie.Error(hub.uiBroadcast.Release(subscriberID))
		}()

		hub.clients.Store(conn, conn)

		defer func() {
			errnie.Error(conn.Close())
		}()

		defer hub.clients.Delete(conn)

		hub.balancesBroadcast.Send(datura.Acquire(
			"hub", datura.APPJSON,
		).WithDestination(
			"kraken:private",
		).WithRole(
			"balances",
		).WithScope(
			"subscribe",
		).WithPayload(datura.Map[any]{
			"method": "subscribe",
			"params": datura.Map[any]{
				"channel": "balances",
			},
		}.Marshal()))

		for {
			artifact, err := subscription.Wait(hub.ctx)

			if errnie.Error(err) != nil {
				return
			}

			if conn.WriteMessage(websocket.BinaryMessage, artifact.Pack()) != nil {
				return
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
