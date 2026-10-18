package broker

import (
	"github.com/gofiber/fiber/v3"
)

/*
Service exposes REST endpoints for broker operations and position lifecycle queries.
Positions and their lifecycles are owned directly by the Desk.
*/
type Service struct {
	desk *Desk
}

/*
NewService constructs a broker HTTP service bound to desk and optional balance.
*/
func NewService(desk *Desk) *Service {
	svc := &Service{
		desk: desk,
	}

	return svc
}

/*
Register mounts the broker endpoints onto router.
*/
func (service *Service) Register(router fiber.Router) {
	if service == nil || service.desk == nil || router == nil {
		return
	}

	router.Get("/positions", func(ctx fiber.Ctx) error {
		positions := make([]*Position, 0)

		service.desk.positions.Range(func(key, value any) bool {
			positions = append(positions, value.(*Position))
			return true
		})

		return ctx.Status(fiber.StatusOK).JSON(positions)
	})

	router.Get("/balance", func(ctx fiber.Ctx) error {
		return ctx.Status(fiber.StatusOK).JSON(
			service.desk.balance,
		)
	})
}
