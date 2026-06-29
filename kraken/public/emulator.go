package public

import (
	"context"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public/response"
	"github.com/theapemachine/symm/kraken/types"
)

/*
Emulator is a local WebSocket server for paper trading, which acts as the kraken:private
socket emulation and simulates as accurately as possible what would happen in a live
trading scenario. This is the only seam where differences betwen live and paper trading
are implemented, and that should always remain so. All other code should be allowed to
be entirely agnostic about which trading model it is currently using.
*/
type Emulator struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	tree          *dmt.Tree
	handlers      map[string]types.Socket
	broadcasts    *sync.Map
	subscriptions *sync.Map
	app           *fiber.App
	listenAddr    string
	clients       sync.Map
}

func NewEmulator(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) (*Emulator, error) {
	ctx, cancel := context.WithCancel(ctx)
	listenAddr := viper.GetString("emulator.addr")

	if listenAddr == "" {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"emulator: listen address is required (emulator.addr)",
			nil,
		))
	}

	emulator := &Emulator{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		tree:   tree,
		handlers: map[string]types.Socket{
			"balances":   response.NewBalances(ctx, pool, tree),
			"orders":     response.NewOrdersWithTree(ctx, pool, tree),
			"executions": response.NewExecutions(ctx),
		},
		broadcasts:    &sync.Map{},
		subscriptions: &sync.Map{},
		listenAddr:    listenAddr,
		app: fiber.New(fiber.Config{
			JSONEncoder:   sonic.Marshal,
			JSONDecoder:   sonic.Unmarshal,
			StrictRouting: true,
		}),
	}

	emulator.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	emulator.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		emulator.clients.Store(conn, conn)

		defer func() {
			errnie.Error(conn.Close())
		}()

		defer emulator.clients.Delete(conn)

		for {
			_, message, err := conn.ReadMessage()

			if errnie.Error(err) != nil {
				continue
			}

			msg := &types.SocketMessage{}

			if err := msg.Decode(message); err != nil {
				errnie.Error(err)
				continue
			}

			handler, ok := emulator.handlers[msg.Channel]

			if !ok {
				continue
			}

			response := handler.Send(datura.Acquire(
				"kraken:private", datura.APPJSON,
			).WithRole(
				msg.Channel,
			).WithScope(
				msg.Type,
			).WithPayload(
				message,
			))

			if response == nil {
				continue
			}

			if errnie.Error(conn.WriteMessage(
				websocket.TextMessage, response.DecryptPayload(),
			)) != nil {
				continue
			}
		}
	}))

	return emulator, nil
}

func (emulator *Emulator) Endpoint() EndpointType {
	return EndpointType("ws://" + emulator.listenAddr + "/ws")
}

func (emulator *Emulator) Serve() error {
	return errnie.Error(emulator.app.Listen(emulator.listenAddr))
}

func (emulator *Emulator) Close() error {
	if emulator.app != nil {
		errnie.Error(emulator.app.Shutdown())
	}

	emulator.cancel()
	return nil
}
