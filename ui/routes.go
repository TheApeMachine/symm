package ui

import (
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/proxy"
	"github.com/theapemachine/errnie"
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

	routes.hub.app.Get("/ws", routes.hub.wsHandler())

	routes.hub.app.Post("/hindsight/commit", func(reqCtx fiber.Ctx) error {
		fn, _ := routes.hub.commit.Load().(CommitFunc)
		if fn == nil {
			return reqCtx.Status(fiber.StatusServiceUnavailable).SendString("[hindsight] commit is not attached")
		}

		payload, err := fn(reqCtx.Body())
		if err != nil {
			status := fiber.StatusInternalServerError
			switch {
			case errnie.IsNotFound(err):
				status = fiber.StatusNotFound
			case errnie.IsKind(err, errnie.Validation), errnie.IsKind(err, errnie.UnprocessableContent):
				status = fiber.StatusUnprocessableEntity
			case errnie.IsKind(err, errnie.NotAcceptable):
				status = fiber.StatusServiceUnavailable
			}

			return reqCtx.Status(status).SendString(err.Error())
		}

		reqCtx.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		return reqCtx.Status(fiber.StatusOK).Send(payload)
	})

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
