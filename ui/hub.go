package ui

import (
	"context"
	"fmt"
	neturl "net/url"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

type Hub struct {
	*runtime.System
	ctx        context.Context
	uiTees     []*UITee
	app        *fiber.App
	listenAddr string
	routes     *Routes
}

/*
NewHub constructs the dashboard hub from its queue-backed system boundaries.
*/
func NewHub(
	ctx context.Context,
	uiTees ...*UITee,
) *Hub {
	hub := &Hub{
		ctx:        ctx,
		uiTees:     uiTees,
		listenAddr: system.Cfg.UI.Addr,
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  system.Cfg.UI.WebSocket.MaxMessageBytes,
			WriteBufferSize: system.Cfg.UI.WebSocket.MaxMessageBytes,
		}),
	}

	hub.routes = NewRoutes(hub)
	hub.System = runtime.NewSystem(ctx, "hub")

	// The dashboard is a separate origin from the hub (vite dev server on
	// :3000 vs. the hub on :8765). Permit loopback origins on any port
	// so a locally-served dashboard can always read it without opening CORS to
	// arbitrary remote origins.
	hub.app.Use(cors.New(cors.Config{
		AllowOriginsFunc: func(origin string) bool {
			parsed, err := neturl.Parse(origin)

			if err != nil {
				return false
			}

			host := parsed.Hostname()

			return host == "localhost" || host == "127.0.0.1" || host == "::1"
		},
		AllowMethods:        []string{"GET", "POST", "HEAD", "OPTIONS"},
		AllowHeaders:        []string{"*"},
		AllowPrivateNetwork: true,
	}))

	hub.routes.Register()

	return hub
}

func (hub *Hub) wsHandler(shard int) fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		errnie.Info(fmt.Sprintf("[hub] frontend websocket shard %d connected", shard))

		defer func() {
			errnie.Info(fmt.Sprintf("[hub] frontend websocket shard %d disconnected", shard))

			if err := conn.Conn.Close(); err != nil {
				errnie.Error(err)
			}
		}()

		go func() {
			for {
				select {
				case <-hub.ctx.Done():
					return
				default:
				}

				_, payload, err := conn.Conn.ReadMessage()

				if err != nil {
					return
				}

				hub.handleCommand(payload)
			}
		}()

		for {
			select {
			case <-hub.ctx.Done():
				return
			default:
			}

			if hub.Status() != runtime.READY {
				continue
			}

			for {
				payload := *(*[]byte)(hub.uiTees[shard].Next())

				if len(payload) == 0 {
					continue
				}

				if err := conn.Conn.WriteMessage(
					websocket.BinaryMessage, payload,
				); err != nil {
					return
				}
			}
		}
	}, websocket.Config{
		Origins: []string{"*"},
	})
}

/*
handleCommand dispatches one inbound JSON command from the dashboard socket.
*/
func (hub *Hub) handleCommand(payload []byte) {
	var request struct {
		Type      string `json:"type"`
		Symbol    string `json:"symbol"`
		Route     string `json:"route"`
		At        string `json:"at"`
		CaptureID int64  `json:"captureId"`
	}

	if err := sonic.Unmarshal(payload, &request); err != nil {
		return
	}

	switch request.Type {
	case "focus":
		types.SetFocus(request.Symbol)
	case "route":
		types.SetRoute(request.Route)
	}
}

/*
App exposes the underlying Fiber application for registering domain services.
*/
func (hub *Hub) App() *fiber.App {
	return hub.app
}

/*
Run listens for dashboard clients on the configured address.
*/
func (hub *Hub) Run() {
	go func() {
		address := hub.listenAddr

		if address == "" {
			address = ":8765"
		}

		hub.app.Listen(address)
	}()
}
