package ui

import (
	"context"
	"errors"
	"strings"
	"syscall"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
Hub owns the dashboard websocket and forwards typed backend frames to clients.
ready gates upgrades until Warmup (or later) so the browser cannot attach
during preflight and inherit allocator/ticker gaps as BACKEND ERROR overlays.
*/
type Hub struct {
	status     types.Status
	ctx        context.Context
	cancel     context.CancelFunc
	app        *fiber.App
	listenAddr string
	Messages   chan []byte
	balance    *broker.Balance
	ready      func() bool
}

func NewHub(
	ctx context.Context,
	price *broker.Price,
	balance *broker.Balance,
	thesis *types.Thesis,
	channel chan []byte,
	ready func() bool,
) (*Hub, error) {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		status:     types.INITIALIZING,
		ctx:        ctx,
		cancel:     cancel,
		listenAddr: viper.GetString("ui.addr"),
		Messages:   channel,
		app: fiber.New(fiber.Config{
			JSONEncoder: sonic.Marshal,
			JSONDecoder: sonic.Unmarshal,
		}),
		balance: balance,
		ready:   ready,
	}

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if !hub.open() {
			return fiber.ErrServiceUnavailable
		}

		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		defer conn.Close()

		// Write the desk snapshot on the socket itself. Publish() is
		// non-blocking into Messages and is dropped when the tick stream has
		// saturated the buffer — exactly the refresh path that left cash and
		// open holdings blank while engine.open still counted desk slots.
		if hub.balance != nil && conn.Conn != nil {
			if frame := hub.balance.Frame(); len(frame) > 0 {
				if err := conn.Conn.WriteMessage(websocket.TextMessage, frame); err != nil {
					if hub.clientGone(err) {
						return
					}

					errnie.Error(err)
					return
				}
			}
		}

		for {
			select {
			case <-hub.ctx.Done():
				return
			case msg := <-hub.Messages:
				// FOR THE FINAL TIME: THERE IS ONLY 1 CLIENT, THE FRONTEND,
				// SO: NO, THERE IS NO SITUATION OF MULTIPLE CLIENTS COMPETING
				// FOR THE SAME MESSAGE CHANNEL!

				if conn.Conn == nil {
					return
				}

				if err := conn.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					if hub.clientGone(err) {
						return
					}

					errnie.Error(err)
					return
				}
			}
		}
	}, websocket.Config{
		EnableCompression: true,
	}))

	return hub, nil
}

/*
open reports whether the dashboard socket may accept clients. A nil ready
callback stays open (tests); production passes Booter.Ready(Warmup).
*/
func (hub *Hub) open() bool {
	if hub == nil || hub.ready == nil {
		return true
	}

	return hub.ready()
}

func (hub *Hub) Initialize() error {
	errnie.Info("initializing UI hub")
	hub.status = types.READY
	return nil
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

/*
clientGone reports write failures that mean the browser dropped or desynced the
socket rather than a hub-side publish bug.
*/
func (hub *Hub) clientGone(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	if websocket.IsCloseError(err) {
		return true
	}

	message := err.Error()

	return strings.Contains(message, "unexpected bytes at end of flate stream") ||
		strings.Contains(message, "use of closed network connection") ||
		strings.Contains(message, "broken pipe")
}
