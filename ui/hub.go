package ui

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

/*
Hub subscribes to the ui broadcast group and forwards frames to the dashboard
websocket client.
*/
type Hub struct {
	ctx         context.Context
	cancel      context.CancelFunc
	subscribers *sync.Map
	client      atomic.Pointer[websocket.Conn]
	app         *fiber.App
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
		ctx:         ctx,
		cancel:      cancel,
		subscribers: &sync.Map{},
		app: fiber.New(fiber.Config{
			JSONEncoder:   sonic.Marshal,
			JSONDecoder:   sonic.Unmarshal,
			StrictRouting: true,
		}),
	}

	for _, channel := range []string{
		"ui",
	} {
		hub.subscribers.Store(
			channel, pool.Subscribe(channel, nil),
		)
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
			message *datura.Artifact
			err     error
		)

		for {
			consumer, ok := hub.subscribers.Load("ui")

			if !ok {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"hub: ui subscriber not found",
					nil,
				))

				return
			}

			message, err = consumer.(*qpool.BroadcastConsumer).Wait(hub.ctx)

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					"hub: failed to wait for message",
					err,
				))

				continue
			}

			writer, err := conn.NextWriter(websocket.TextMessage)

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					"hub: failed to get next writer",
					err,
				))

				continue
			}

			if _, err := writer.Write([]byte(message.Peek("payload"))); err != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					"hub: failed to write websocket payload",
					err,
				))

				writer.Close()

				continue
			}

			if err := writer.Close(); err != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					"hub: failed to close writer",
					err,
				))

				continue
			}
		}
	}))

	if err := hub.app.Listen(listenAddr, fiber.ListenConfig{
		EnablePrefork: false,
	}); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"hub: failed to listen",
			err,
		))
	}

	return hub
}

/*
PublishMeasurements ships one state frame with every live measurement to ui subscribers.
*/
func PublishMeasurements(
	pool *qpool.Q[any],
	measurements []logic.Measurement,
) error {
	if len(measurements) == 0 {
		return nil
	}

	payload, err := sonic.Marshal(map[string]any{
		"type":         "state",
		"measurements": measurements,
	})

	if err != nil {
		return errnie.Err(
			errnie.Validation,
			"hub: failed to marshal measurement state",
			err,
		)
	}

	artifact := datura.Acquire("ui", datura.Artifact_Type_json).
		WithRole("state").
		WithDestination("ui")

	if setErr := artifact.SetPayload(payload); setErr != nil {
		return errnie.Err(
			errnie.Validation,
			"hub: failed to set measurement state payload",
			setErr,
		)
	}

	return pool.CreateBroadcastGroup("ui").Send(artifact)
}

func (hub *Hub) Close() error {
	if hub.app != nil {
		errnie.Error(hub.app.Shutdown())
	}

	hub.cancel()
	return nil
}
