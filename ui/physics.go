package ui

import (
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

// Reads are bounded observations. They never dispatch a physics step.
func (hub *Hub) registerPhysics() {
	hub.app.Get("/physics", func(c fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(sensorium.PhysicsMonitorHTML)
	})
	hub.app.Get("/physics/health", func(c fiber.Ctx) error {
		b, status := hub.physics.Poll()
		c.Set("Cache-Control", "no-store")
		c.Set("Content-Type", "application/json")
		return c.Status(status).Send(b)
	})
}
