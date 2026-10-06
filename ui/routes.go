package ui

import (
	"encoding/json"
	"strconv"

	"github.com/apache/iceberg-go"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/signal"
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

	routes.hub.app.Get("/cognition/tree", func(c fiber.Ctx) error {
		if routes.hub.cognitionSource == nil {
			return c.JSON(CognitionTreeExport{})
		}

		export := routes.hub.cognitionSource.CognitionTree()
		return c.JSON(export)
	})

	routes.hub.app.Get("/training/fragments", func(c fiber.Ctx) error {
		if routes.hub.fragmentsSource == nil {
			return c.JSON([]TrainedFragment{})
		}

		fragments := routes.hub.fragmentsSource.Fragments()

		if fragments == nil {
			fragments = []TrainedFragment{}
		}

		return c.JSON(fragments)
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
		batch := make([]*data.Measurement, 0, timelineBatchSize)

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
		symbols := []string{}
		seen := make(map[string]bool)
		for measurement := range routes.hub.store.Scan(routes.hub.Context(), tables.Measurements, epoch, nil, 0) {
			if measurement != nil && measurement.Label != "" && !seen[measurement.Label] {
				seen[measurement.Label] = true
				symbols = append(symbols, measurement.Label)
			}
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
		excursions := []*data.Measurement{}

		for measurement := range routes.hub.store.Scan(routes.hub.Context(), tables.Measurements, epoch, nil, 0) {
			if status := measurement.Meta("status"); status == "resolved" {
				excursions = append(excursions, measurement)
			}
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
			tableName = tables.Measurements
		}

		limit := int(parseUintQuery(ctx.Query("limit")))
		var measurements []*data.Measurement

		for measurement := range routes.hub.store.Scan(routes.hub.Context(), tableName, epoch, nil, limit) {
			measurements = append(measurements, measurement)
		}

		if measurements == nil {
			measurements = []*data.Measurement{}
		}

		return ctx.JSON(measurements)
	})

	// Captures: bounded SpotTicker identities for FrameStrip / CaptureCard.
	// Full research timelines go through DuckDB/workbench, not this listing.
	routes.hub.app.Get("/hindsight/captures", func(ctx fiber.Ctx) error {
		if routes.hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		run := ctx.Query("run")
		if run == "" {
			run = ctx.Query("epoch")
		}
		epoch := parseInt64Query(run)
		after := parseInt64Query(ctx.Query("after"))
		symbol := ctx.Query("symbol")

		// Bounded SpotTicker scan only — research timelines use DuckDB/workbench;
		// this neighbourhood listing must not pull every peer family through Timeline.
		const maxCaptures = 64
		captures := make([]map[string]any, 0, maxCaptures)

		var filter iceberg.BooleanExpression
		if symbol != "" {
			filter = iceberg.EqualTo(iceberg.Reference("symbol"), symbol)
		}
		if after > 0 {
			expression := iceberg.BooleanExpression(iceberg.GreaterThanEqual(iceberg.Reference("tick"), after))
			if filter != nil {
				expression = iceberg.NewAnd(filter, expression)
			}
			filter = expression
		}

		for measurement := range routes.hub.store.Scan(routes.hub.Context(), tables.Measurements, epoch, filter, maxCaptures) {
			if measurement == nil || measurement.SeqIdx < after {
				continue
			}

			receivedAt := ""
			if !measurement.At.IsZero() {
				receivedAt = measurement.At.UTC().Format("2006-01-02T15:04:05.000Z07:00")
			}

			captures = append(captures, map[string]any{
				"identity": map[string]any{
					"run":            strconv.FormatInt(epoch, 10),
					"sequence":       measurement.SeqIdx,
					"stream":         measurement.Source,
					"streamEpoch":    epoch,
					"streamSequence": measurement.SeqIdx,
				},
				"kind":       "ticker",
				"endpoint":   measurement.Source,
				"receivedAt": receivedAt,
			})

			if len(captures) >= maxCaptures {
				break
			}
		}

		return ctx.JSON(captures)
	})

	// Envelope: honest projection of the Iceberg frame at seq — metrics + peers.
	// No invented witnesses/manifests when the store never recorded them.
	routes.hub.app.Get("/hindsight/envelope", func(ctx fiber.Ctx) error {
		if routes.hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		run := ctx.Query("run")
		if run == "" {
			run = ctx.Query("epoch")
		}
		epoch := parseInt64Query(run)
		seq := parseInt64Query(ctx.Query("seq"))
		symbol := ctx.Query("symbol")

		if seq <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, "seq required")
		}

		var found *data.Measurement

		for measurement := range routes.hub.store.Timeline(routes.hub.Context(), epoch, symbol, seq, seq) {
			if measurement == nil {
				continue
			}
			if measurement.SeqIdx == seq {
				found = measurement
				break
			}
		}

		if found == nil {
			return fiber.NewError(fiber.StatusNotFound, "frame not found")
		}

		payloadBytes, err := json.Marshal(found)
		if err != nil {
			return err
		}

		receivedAt := ""
		if !found.At.IsZero() {
			receivedAt = found.At.UTC().Format("2006-01-02T15:04:05.000Z07:00")
		}

		capture := map[string]any{
			"identity": map[string]any{
				"run":            strconv.FormatInt(epoch, 10),
				"sequence":       found.SeqIdx,
				"stream":         found.Source,
				"streamEpoch":    epoch,
				"streamSequence": found.SeqIdx,
			},
			"kind":       "ticker",
			"endpoint":   found.Source,
			"receivedAt": receivedAt,
		}

		return ctx.JSON(map[string]any{
			"run":       strconv.FormatInt(epoch, 10),
			"sequence":  found.SeqIdx,
			"capture":   capture,
			"payload":   string(payloadBytes),
			"manifests": []any{},
			"witnesses": []any{},
		})
	})

	// Honest empties: these witness streams are not persisted in Iceberg yet.
	routes.hub.app.Get("/hindsight/gaps", func(ctx fiber.Ctx) error {
		return ctx.JSON([]any{})
	})
	routes.hub.app.Get("/hindsight/lifecycle", func(ctx fiber.Ctx) error {
		return ctx.JSON([]any{})
	})
	routes.hub.app.Get("/hindsight/states", func(ctx fiber.Ctx) error {
		return ctx.JSON([]any{})
	})
	routes.hub.app.Get("/hindsight/state", func(ctx fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotFound, "no witnessed state at sequence")
	})
}
