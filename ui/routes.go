package ui

import (
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/proxy"
	"github.com/theapemachine/symm/hindsight"
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

	/*
		The hindsight REST (run list, scan, commit) is the label boundary the
		dashboard draws on: commits write detections/, and symm train reads
		nothing else. It mounts here so it shares the hub's origin and store.
	*/
	if routes.hub.client != nil {
		service, err := hindsight.NewService(routes.hub.ctx, routes.hub.client)

		if err != nil {
			routes.hub.Error(err)
		} else {
			service.Register(routes.hub.app)
		}
	}

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
