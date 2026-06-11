package ui

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
)

/*
Hub subscribes to the ui broadcast group and forwards frames to the dashboard
websocket client.
*/
type Hub struct {
	ctx    context.Context
	cancel context.CancelFunc
	bus    *internal.Bus
	client atomic.Pointer[websocket.Conn]
	app    *fiber.App
}

func NewHub(
	ctx context.Context,
	pool *qpool.Q[any],
) *Hub {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		ctx:    ctx,
		cancel: cancel,
		bus: internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelKrakenPrivate},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelUI, "ui:hub"),
			},
		),
		app: fiber.New(),
	}

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		var (
			message *qpool.QValue[any]
			err     error
		)

		hub.bus.Send(internal.ChannelKrakenPrivate, "balances", types.KrakenMessage{
			Method: "subscribe",
			Params: user.BalanceParams{
				Channel:  public.BalancesChannel,
				Snapshot: true,
			},
		})

		for {
			if message, err = hub.bus.Receive(
				internal.ChannelUI,
			); errnie.Error(err) != nil || message == nil {
				continue
			}

			frame, frameErr := uiWireFrame(message)

			if frameErr != nil {
				errnie.Error(frameErr)
				continue
			}

			if err = conn.WriteJSON(frame); err != nil {
				errnie.Error(err)
				break
			}
		}
	}))

	go func() {
		if err := hub.app.Listen("0.0.0.0:8765"); err != nil {
			errnie.Error(err)
		}
	}()

	return hub
}

func uiWireFrame(message *qpool.QValue[any]) (map[string]any, error) {
	frame := map[string]any{}

	if message == nil {
		return nil, errnie.Error(errnie.Require(map[string]any{
			"message": message,
		}))
	}

	encoded, err := json.Marshal(message.Value)

	if err != nil {
		return nil, errnie.Error(err)
	}

	if err = json.Unmarshal(encoded, &frame); err != nil {
		return map[string]any{
			"type":  message.Type,
			"value": message.Value,
		}, nil
	}

	frame["type"] = message.Type

	return frame, nil
}

func (hub *Hub) Close() error {
	if hub.app != nil {
		errnie.Error(hub.app.Shutdown())
	}

	hub.cancel()
	return nil
}
