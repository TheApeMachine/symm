package ui

import (
	"context"
	"errors"
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
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q[any]
	tree        *dmt.Tree
	uiBroadcast *qpool.BroadcastGroup
	broadcasts  *sync.Map
	app         *fiber.App
	listenAddr  string
	clients     sync.Map
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
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		tree:        tree,
		uiBroadcast: pool.CreateBroadcastGroup("ui"),
		broadcasts:  &sync.Map{},
		listenAddr:  listenAddr,
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

		go func() {
			errnie.Error(hub.SubscribeInstruments())
		}()

		var (
			latest      *datura.Artifact
			latestStamp int64
		)

		for candidate := range hub.tree.Seek([]byte("balances/")) {
			// Sort the balances by timestamp and send the latest one to the client
			if candidate.Timestamp() >= latestStamp {
				latest = candidate
				latestStamp = candidate.Timestamp()
			}
		}

		if latest != nil {
			if conn.WriteMessage(
				websocket.BinaryMessage, latest.Pack(),
			) != nil {
				return
			}
		}

		for {
			artifact, err := subscription.Wait(hub.ctx)
			if err != nil {
				return
			}

			if conn.WriteMessage(
				websocket.BinaryMessage, artifact.Pack(),
			) != nil {
				return
			}
		}
	}))

	return hub, nil
}

func (hub *Hub) Run() error {
	if err := hub.app.Listen(hub.listenAddr, fiber.ListenConfig{
		EnablePrefork: false,
	}); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"hub: failed to listen",
			err,
		))
	}

	return nil
}

func (hub *Hub) Close() error {
	errnie.Error(hub.UnSubscribeInstruments())

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

	// Only the instrument channel is requested here. Its snapshot drives
	// ticker/book/trade subscriptions per discovered pair in kraken/public.
	if err := bg.(*qpool.BroadcastGroup).Send(datura.Acquire(
		"hub", datura.APPJSON,
	).WithDestination(
		"kraken:public",
	).WithPayload(
		[]byte(`{"method": "subscribe","params": {"channel": "instrument", "snapshot": true}}`))); err != nil {
		return errnie.Error(err)
	}

	return nil
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

	// Close tears down the connection, dropping every data subscription with
	// it, so only the instrument channel needs an explicit unsubscribe.
	if err := bg.(*qpool.BroadcastGroup).Send(datura.Acquire(
		"hub", datura.APPJSON,
	).WithDestination(
		"kraken:public",
	).WithPayload(
		[]byte(`{"method": "unsubscribe","params": {"channel": "instrument"}}`))); err != nil {
		return errnie.Error(err)
	}

	return nil
}
