package ui

import (
	"context"
	"errors"
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
	ctx            context.Context
	cancel         context.CancelFunc
	tree           *dmt.Tree
	uiBroadcast    *qpool.BroadcastGroup
	broadcasts     *sync.Map
	uiSubscription *qpool.BroadcastConsumer
	app            *fiber.App
	listenAddr     string
	clients        sync.Map
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
		broadcasts:     &sync.Map{},
		listenAddr:     listenAddr,
		app: fiber.New(fiber.Config{
			JSONEncoder:   sonic.Marshal,
			JSONDecoder:   sonic.Unmarshal,
			StrictRouting: true,
		}),
	}

	for _, channel := range []string{"kraken:public"} {
		hub.broadcasts.Store(channel, pool.CreateBroadcastGroup(channel))
	}

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

		hub.SubscribeInstruments()

		for {
			artifact, err := hub.uiSubscription.Wait(hub.ctx)

			if errnie.Error(err) != nil {
				return
			}

			writer, err := conn.NextWriter(websocket.BinaryMessage)

			if errnie.Error(err) != nil {
				return
			}

			_, err = writer.Write(artifact.Pack())

			if errnie.Error(err) != nil {
				return
			}

			if errnie.Error(writer.Close()) != nil {
				return
			}
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

func (hub *Hub) SubscribeInstruments() error {
	bg, ok := hub.broadcasts.Load("kraken:public")

	if !ok || bg == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/public: failed to load broadcast group",
			errors.New("kraken:public"),
		))
	}

	return errnie.Error(bg.(*qpool.BroadcastGroup).Send(datura.Acquire(
		"hub", datura.APPJSON,
	).WithDestination(
		"kraken:public",
	).WithPayload(
		[]byte(`{"method": "subscribe","params": {"channel": "instrument", "snapshot": true}}`))),
	)
}

func (hub *Hub) UnSubscribeInstruments() error {
	bg, ok := hub.broadcasts.Load("kraken:public")

	if !ok || bg == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/public: failed to load broadcast group",
			errors.New("kraken:public"),
		))
	}

	return errnie.Error(bg.(*qpool.BroadcastGroup).Send(datura.Acquire(
		"hub", datura.APPJSON,
	).WithDestination(
		"kraken:public",
	).WithPayload(
		[]byte(`{"method": "unsubscribe","params": {"channel": "instrument"}}`),
	)))
}
