package ui

import (
	"context"
	"errors"
	"io"
	"sync"
	"syscall"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
)

var cacheKeys = []string{"balances", "executions", "instruments", "positions", "tick"}

/*
Hub owns the dashboard websocket and forwards typed backend frames to clients.
*/
type Hub struct {
	ctx         context.Context
	cancel      context.CancelFunc
	app         *fiber.App
	listenAddr  string
	Messages    chan []byte
	price       *broker.Price
	balance     *broker.Balance
	subscribers *sync.Map
	focus       func(string)
}

func NewHub(
	ctx context.Context,
	price *broker.Price,
	balance *broker.Balance,
	channel chan []byte,
) (*Hub, error) {
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
		balance:     balance,
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
		defer conn.Close()
		go hub.read(conn)

		if hub.balance != nil {
			if frame := hub.balance.Frame(); len(frame) > 0 {
				if err := conn.Conn.WriteMessage(websocket.TextMessage, frame); err != nil {
					if errors.Is(err, syscall.EPIPE) {
						return
					}

					errnie.Error(err)
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
					for _, closeError := range []error{
						syscall.EPIPE,
						syscall.ECONNRESET,
						io.EOF,
						io.ErrClosedPipe,
					} {
						if errors.Is(err, closeError) {
							return
						}
					}

					errnie.Error(err)
					return
				}
			}
		}
	}))

	return hub, nil
}

/*
BindFocus connects browser symbol selection to the analysis projection owner.
The stack binds this before Serve, so full field data is produced only for the
one symbol the dashboard actually requests.
*/
func (hub *Hub) BindFocus(focus func(string)) {
	hub.focus = focus
}

/*
read consumes browser control messages independently from the outbound frame
loop. Websocket permits one concurrent reader and writer; keeping those roles
separate prevents a quiet market from blocking a focus change.
*/
func (hub *Hub) read(conn *websocket.Conn) {
	for {
		_, message, err := conn.Conn.ReadMessage()

		if err != nil {
			return
		}

		if err := hub.focusMessage(message); err != nil {
			errnie.Error(err)
		}
	}
}

/*
focusMessage validates one browser control frame before changing analysis
focus. Invalid or unbound commands are errors rather than silent no-ops because
without this link the backend intentionally omits the expensive field payload.
*/
func (hub *Hub) focusMessage(message []byte) error {
	command := struct {
		Focus string `json:"focus"`
	}{}

	if err := sonic.Unmarshal(message, &command); err != nil {
		return errnie.Err(errnie.Validation, "ui hub: invalid focus command", err)
	}

	if command.Focus == "" {
		return errnie.Err(errnie.Validation, "ui hub: focus symbol is empty", nil)
	}

	if hub.focus == nil {
		return errnie.Err(errnie.Internal, "ui hub: focus handler is not bound", nil)
	}

	hub.focus(command.Focus)
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
