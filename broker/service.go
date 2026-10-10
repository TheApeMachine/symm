package broker

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
)

/*
BalanceReport summarizes account cash, unrealized PnL, total equity, and asset holdings.
*/
type BalanceReport struct {
	Quote      string            `json:"quote,omitempty"`
	Cash       string            `json:"cash"`
	Unrealized string            `json:"unrealized"`
	Equity     string            `json:"equity"`
	Assets     map[string]string `json:"assets,omitempty"`
}

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

	router.Get("/positions", service.positions)
	router.Get("/balance", service.balanceReport)
	router.Get("/equity", service.balanceReport)
}

func (service *Service) positions(reqCtx fiber.Ctx) error {
	positions := make([]*Position, 0)

	service.desk.positions.Range(func(key, value any) bool {
		positions = append(positions, value.(*Position))
		return true
	})

	return reqCtx.JSON(positions)
}

func (service *Service) balanceReport(reqCtx fiber.Ctx) error {
	balance := service.desk.balance

	if balance == nil {
		return reqCtx.JSON(BalanceReport{
			Cash:       "0",
			Unrealized: "0",
			Equity:     "0",
		})
	}

	cash := "0"
	unrealized := "0"
	equity := "0"

	if balance.measurement != nil {
		for entry := range balance.measurement.Read("cash") {
			if entry.Metric != nil {
				cash = strconv.FormatFloat(entry.Metric.Raw, 'f', -1, 64)
				break
			}
		}

		for entry := range balance.measurement.Read("unrealized") {
			if entry.Metric != nil {
				unrealized = strconv.FormatFloat(entry.Metric.Raw, 'f', -1, 64)
				break
			}
		}

		for entry := range balance.measurement.Read("equity") {
			if entry.Metric != nil {
				equity = strconv.FormatFloat(entry.Metric.Raw, 'f', -1, 64)
				break
			}
		}
	}

	return reqCtx.JSON(BalanceReport{
		Quote:      balance.Quote,
		Cash:       cash,
		Unrealized: unrealized,
		Equity:     equity,
	})
}
