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
ConnectSnapshot supplies dashboard frames written once when a browser connects.
*/
type ConnectSnapshot func() []map[string]any

/*
Hub subscribes to the ui broadcast group and forwards frames to the dashboard
websocket client.
*/
type Hub struct {
	ctx             context.Context
	cancel          context.CancelFunc
	subscribers     *sync.Map
	client          atomic.Pointer[websocket.Conn]
	app             *fiber.App
	connectSnapshot ConnectSnapshot
}

func NewHub(
	ctx context.Context,
	pool *qpool.Q[any],
	connectSnapshot ConnectSnapshot,
) *Hub {
	ctx, cancel := context.WithCancel(ctx)
	listenAddr := viper.GetString("ui.addr")

	if listenAddr == "" {
		listenAddr = "127.0.0.1:8765"
	}

	hub := &Hub{
		ctx:             ctx,
		cancel:          cancel,
		subscribers:     &sync.Map{},
		connectSnapshot: connectSnapshot,
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
		if writeErr := hub.writeConnectSnapshot(conn); writeErr != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"hub: failed to write connect snapshot",
				writeErr,
			))

			return
		}

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

				return
			}

			wirePayload, payloadErr := wireArtifactPayload(message)

			if payloadErr != nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"hub: failed to read websocket payload",
					payloadErr,
				))

				_ = writer.Close()

				continue
			}

			if _, err := writer.Write(wirePayload); err != nil {
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

func (hub *Hub) writeConnectSnapshot(conn *websocket.Conn) error {
	if hub.connectSnapshot == nil {
		return nil
	}

	for _, frame := range hub.connectSnapshot() {
		wirePayload, marshalErr := sonic.Marshal(frame)

		if marshalErr != nil {
			return errnie.Err(
				errnie.Validation,
				"hub: failed to marshal connect snapshot frame",
				marshalErr,
			)
		}

		if writeErr := conn.WriteMessage(websocket.TextMessage, wirePayload); writeErr != nil {
			return errnie.Err(
				errnie.IO,
				"hub: failed to write connect snapshot frame",
				writeErr,
			)
		}
	}

	return nil
}

func wireArtifactPayload(artifact *datura.Artifact) ([]byte, error) {
	payload, err := artifact.DecryptPayload()

	if err != nil {
		return nil, err
	}

	if len(payload) == 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"hub: artifact payload is empty",
			nil,
		)
	}

	return payload, nil
}

/*
PublishMeasurements ships one state frame with live measurements and gauge evidence.
*/
func PublishMeasurements(
	pool *qpool.Q[any],
	measurements []logic.Measurement,
	storyTicks uint64,
) error {
	payload, err := sonic.Marshal(map[string]any{
		"type":           "state",
		"story_ticks":    storyTicks,
		"measurements":   measurements,
		"gauge_readings": gaugeReadingsFromMeasurements(measurements),
	})

	if err != nil {
		return errnie.Err(
			errnie.Validation,
			"hub: failed to marshal measurement state",
			err,
		)
	}

	return pool.CreateBroadcastGroup("ui").Send(datura.Acquire(
		"ui", datura.Artifact_Type_json,
	).WithRole(
		"state",
	).WithDestination(
		"ui",
	).WithPayload(
		payload,
	))
}

/*
PublishPayload ships one dashboard frame to ui subscribers.
*/
func PublishPayload(
	pool *qpool.Q[any],
	role string,
	payload map[string]any,
) error {
	if payload == nil {
		return nil
	}

	marshaled, err := sonic.Marshal(payload)

	if err != nil {
		return errnie.Err(
			errnie.Validation,
			"hub: failed to marshal dashboard payload",
			err,
		)
	}

	return pool.CreateBroadcastGroup("ui").Send(datura.Acquire(
		"ui", datura.Artifact_Type_json,
	).WithRole(
		role,
	).WithDestination(
		"ui",
	).WithPayload(
		marshaled,
	))
}

func (hub *Hub) Close() error {
	if hub.app != nil {
		errnie.Error(hub.app.Shutdown())
	}

	hub.cancel()
	return nil
}
