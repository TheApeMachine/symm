package ui

import (
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/signal"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

type Routes struct {
	hub *Hub
}

func NewRoutes(hub *Hub) *Routes {
	return &Routes{hub: hub}
}

func (routes *Routes) Register() {
	routes.hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	routes.hub.app.Get("/trades", func(c fiber.Ctx) error {
		if routes.hub.tradeStore == nil {
			return c.JSON([]*wire.PositionT{})
		}

		trades, err := routes.hub.tradeStore.RecentTrades(int(
			min(parseUintQuery(c.Query("limit")), 2000),
		))

		if err != nil {
			return err
		}

		if trades == nil {
			trades = []*wire.PositionT{}
		}

		return c.JSON(trades)
	})

	routes.hub.app.Get("/cognition/tree", func(c fiber.Ctx) error {
		if routes.hub.cognitionSource == nil {
			return c.JSON(cognition.CognitionTreeExport{
				Root: &cognition.TrieNodeJSON{
					ID:          "root",
					TokenPrefix: "ROOT",
					Probability: 1.0,
					State:       "ESTIMATED",
				},
				Branches: []cognition.TrieBranchJSON{},
				Feasible: []cognition.FeasibleActionJSON{},
			})
		}

		export := routes.hub.cognitionSource.CognitionTree()
		return c.JSON(export)
	})

	// Hindsight inspection projection reads
	routes.hub.app.Get("/hindsight/metric-map", func(c fiber.Ctx) error {
		return c.JSON(signal.Semantics())
	})

	routes.hub.app.Get("/hindsight/runs", func(c fiber.Ctx) error {
		if routes.hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		runs, err := routes.hub.store.Runs(routes.hub.Context())

		if err != nil {
			return err
		}

		if runs == nil {
			runs = []tables.Run{}
		}

		return c.JSON(runs)
	})

	routes.hub.app.Use("/hindsight/timeline", func(ctx fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(ctx) {
			ctx.Locals("allowed", true)
			return ctx.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	routes.hub.app.Get("/hindsight/timeline", websocket.New(func(conn *websocket.Conn) {
		if routes.hub.store == nil {
			return
		}

		run := conn.Query("run")

		if run == "" {
			run = conn.Query("epoch")
		}

		if run == "" {
			return
		}

		epoch := parseInt64Query(run)
		symbol := conn.Query("symbol")
		fromTick := parseInt64Query(conn.Query("from"))
		toTick := parseInt64Query(conn.Query("to"))

		const timelineBatchSize = 256
		batch := make([]*data.Measurement[float64], 0, timelineBatchSize)

		for measurement := range routes.hub.store.Timeline(routes.hub.Context(), epoch, symbol, fromTick, toTick) {
			batch = append(batch, measurement)

			if len(batch) < timelineBatchSize {
				continue
			}

			payload, err := types.EncodeMeasurements(batch)

			if err != nil {
				return
			}

			if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
				return
			}

			batch = batch[:0]
		}

		if len(batch) > 0 {
			payload, err := types.EncodeMeasurements(batch)

			if err != nil {
				return
			}

			if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
				return
			}
		}
	}, websocket.Config{
		Origins: []string{"*"},
	}))

	routes.hub.app.Get("/hindsight/symbols", func(ctx fiber.Ctx) error {
		if routes.hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		run := ctx.Query("run")

		if run == "" {
			run = ctx.Query("epoch")
		}

		epoch := parseInt64Query(run)
		symbols, err := routes.hub.store.Symbols(routes.hub.Context(), epoch)

		if err != nil {
			return err
		}

		if symbols == nil {
			symbols = []string{}
		}

		return ctx.JSON(symbols)
	})

	routes.hub.app.Get("/hindsight/excursions", func(ctx fiber.Ctx) error {
		if routes.hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		run := ctx.Query("run")

		if run == "" {
			run = ctx.Query("epoch")
		}

		epoch := parseInt64Query(run)
		excursions, err := routes.hub.store.Excursions(routes.hub.Context(), epoch, nil)

		if err != nil {
			return err
		}

		if excursions == nil {
			excursions = []tables.ExcursionRecord{}
		}

		return ctx.JSON(excursions)
	})

	routes.hub.app.Get("/hindsight/data", func(ctx fiber.Ctx) error {
		if routes.hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		epoch := parseInt64Query(ctx.Query("epoch"))
		tableName := ctx.Query("table")

		if tableName == "" {
			tableName = tables.SpotTicker
		}

		limit := int(parseUintQuery(ctx.Query("limit")))
		var measurements []*data.Measurement[float64]

		for measurement := range routes.hub.store.Scan(routes.hub.Context(), tableName, epoch, nil, limit) {
			measurements = append(measurements, measurement)
		}

		if measurements == nil {
			measurements = []*data.Measurement[float64]{}
		}

		return ctx.JSON(measurements)
	})
}
