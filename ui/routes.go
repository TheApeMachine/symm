package ui

import (
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/proxy"
	"github.com/theapemachine/symm/system"
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

	routes.hub.app.Post("/workbench/query", func(reqCtx fiber.Ctx) error {
		targetURL := "http://127.0.0.1:8081/workbench/query"

		if system.Cfg != nil && system.Cfg.Workbench != nil && system.Cfg.Workbench.URL != "" {
			targetURL = system.Cfg.Workbench.URL
		}

		origin := reqCtx.Get("Origin")

		if err := proxy.Do(reqCtx, targetURL); err != nil {
			return fiber.NewError(
				fiber.StatusServiceUnavailable,
				"workbench service unavailable — start standalone workbench with 'make workbench'",
			)
		}

		if origin != "" {
			reqCtx.Set("Access-Control-Allow-Origin", origin)
			reqCtx.Set("Vary", "Origin")
		}

		return nil
	})
}
