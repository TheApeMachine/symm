package ui

import (
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
)

type Routes struct {
	hub *Hub
}

func NewRoutes(hub *Hub) *Routes {
	return &Routes{hub: hub}
}

func (routes *Routes) Register() {
	routes.hub.app.Use("/ws", func(reqCtx fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(reqCtx) {
			reqCtx.Locals("allowed", true)
			return reqCtx.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	routes.hub.app.Get("/ws", routes.hub.wsHandler(0))
	routes.hub.app.Get("/ws/0", routes.hub.wsHandler(0))
	routes.hub.app.Get("/ws/1", routes.hub.wsHandler(1))
}
