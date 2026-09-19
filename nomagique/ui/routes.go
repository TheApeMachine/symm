package ui

import (
	"bytes"
	"io"
	"net/http"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/nomagique/store/tables"
	"github.com/theapemachine/symm/signal"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
arrowStream is the media type of an Apache Arrow IPC stream. The dashboard
hands the response body to the Perspective viewer unparsed, so the body is the
stream itself rather than a JSON envelope carrying it.
*/
const arrowStream = "application/vnd.apache.arrow.stream"

type fluidOffer struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

type Routes struct {
	hub *Hub
}

func NewRoutes(hub *Hub) *Routes {
	routes := &Routes{
		hub: hub,
	}

	hub.app.Get("/trades", func(c fiber.Ctx) error {
		if hub.tradeStore == nil {
			return c.JSON([]*wire.PositionT{})
		}

		limit, err := strconv.ParseUint(c.Query("limit"), 10, 64)

		if err != nil {
			limit = 2000
		}

		trades, err := hub.tradeStore.RecentTrades(int(limit))

		if err != nil {
			hub.Error(errnie.Err(
				errnie.ServiceUnavailable,
				"[hub] failed to get recent trades from the trade store",
				err,
			))

			return c.SendStatus(fiber.StatusServiceUnavailable)
		}

		return c.JSON(trades)
	})

	// Hindsight inspection projection reads
	hub.app.Get("/hindsight/metric-map", func(c fiber.Ctx) error {
		return c.JSON(signal.Semantics())
	})

	hub.app.Get("/hindsight/runs", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		runs, err := hub.store.Runs(hub.Context())

		if err != nil {
			return err
		}

		if runs == nil {
			runs = []tables.Run{}
		}

		return c.JSON(runs)
	})

	hub.app.Use("/hindsight/timeline", func(ctx fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(ctx) {
			ctx.Locals("allowed", true)
			return ctx.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/hindsight/timeline", websocket.New(func(conn *websocket.Conn) {
		if hub.store == nil {
			hub.Error(errnie.Err(
				errnie.ServiceUnavailable,
				"[hub] hindsight store currently not available",
				nil,
			))

			conn.WriteJSON(map[string]any{
				"error": "hindsight store currently not available",
			})

			return
		}

		run := conn.Query("run")

		if run == "" {
			run = conn.Query("epoch")
		}

		if run == "" {
			hub.Error(errnie.Err(
				errnie.BadRequest,
				"[hub] must provide an epoch or run to query the timeline",
				nil,
			))

			conn.WriteJSON(map[string]any{
				"error": "must provide an epoch or run to query the timeline",
			})

			return
		}

		epoch, err := strconv.ParseInt(run, 10, 64)

		if err != nil {
			conn.WriteJSON(map[string]any{
				"error": "must provide an epoch or run to query the timeline",
			})

			return
		}

		symbol := conn.Query("symbol")
		fromTick, err := strconv.ParseInt(conn.Query("from"), 10, 64)

		if err != nil {
			conn.WriteJSON(map[string]any{
				"error": "must provide a valid from to query the timeline",
			})

			return
		}

		toTick, err := strconv.ParseInt(conn.Query("to"), 10, 64)

		if err != nil {
			conn.WriteJSON(map[string]any{
				"error": "must provide a valid to to query the timeline",
			})

			return
		}

		const timelineBatchSize = 256

		for _ = range hub.store.Timeline(hub.Context(), epoch, symbol, fromTick, toTick) {
		}
	}, websocket.Config{
		Origins: []string{"*"},
	}))

	hub.app.Get("/hindsight/symbols", func(ctx fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		run := ctx.Query("run")

		if run == "" {
			run = ctx.Query("epoch")
		}

		epoch, err := strconv.ParseInt(run, 10, 64)

		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid epoch")
		}

		symbols, err := hub.store.Symbols(hub.Context(), epoch)

		if err != nil {
			return err
		}

		if symbols == nil {
			symbols = []string{}
		}

		return ctx.JSON(symbols)
	})

	hub.app.Get("/hindsight/excursions", func(ctx fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		run := ctx.Query("run")

		if run == "" {
			run = ctx.Query("epoch")
		}

		epoch, err := strconv.ParseInt(run, 10, 64)

		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid epoch")
		}

		excursions, err := hub.store.Excursions(hub.Context(), epoch, nil)

		if err != nil {
			return err
		}

		if excursions == nil {
			excursions = []tables.ExcursionRecord{}
		}

		return ctx.JSON(excursions)
	})

	hub.app.Get("/hindsight/data", func(ctx fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
		}

		tableName := ctx.Query("table")

		if tableName == "" {
			tableName = tables.SpotTicker
		}

		return ctx.JSON(map[string]any{})
	})

	hub.app.Get("/workbench/primitives", func(c fiber.Ctx) error {
		primitives, err := compiler.Primitives()

		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.JSON(primitives)
	})


	hub.app.Get("/workbench/signals", func(ctx fiber.Ctx) error {
		ids, err := signal.ListDefinitions()

		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return ctx.JSON(ids)
	})

	hub.app.Get("/workbench/signals/:id", func(ctx fiber.Ctx) error {
		id := ctx.Params("id")
		rawJSON, err := signal.GetDefinition(id)

		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}

		ctx.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		return ctx.Send(rawJSON)
	})

	hub.app.Post("/workbench/signals/:id", func(ctx fiber.Ctx) error {
		id := ctx.Params("id")
		body := ctx.Body()

		if err := signal.SaveDefinition(id, body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		return ctx.SendStatus(fiber.StatusOK)
	})

	// /workbench/query proxies one analytical statement to the standalone
	// workbench service, returning its result as an Arrow IPC stream.
	hub.app.Post("/workbench/query", func(c fiber.Ctx) error {
		var request struct {
			SQL string `json:"sql"`
		}

		if err := c.Bind().Body(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		if request.SQL == "" {
			return fiber.NewError(fiber.StatusBadRequest, "query is empty")
		}

		workbenchURL := viper.GetString("workbench.url")

		if workbenchURL == "" {
			workbenchURL = "http://127.0.0.1:8081/workbench/query"
		}

		payload, err := sonic.Marshal(request)

		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		proxyRequest, err := http.NewRequestWithContext(
			hub.Context(),
			http.MethodPost,
			workbenchURL,
			bytes.NewReader(payload),
		)

		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		proxyRequest.Header.Set("Content-Type", "application/json")

		response, err := http.DefaultClient.Do(proxyRequest)

		if err != nil {
			return fiber.NewError(
				fiber.StatusServiceUnavailable,
				"workbench service unavailable at "+workbenchURL+": "+err.Error(),
			)
		}

		defer func() {
			if closeErr := response.Body.Close(); closeErr != nil {
				errnie.Error(closeErr)
			}
		}()

		if response.StatusCode != http.StatusOK {
			messageBytes, readErr := io.ReadAll(response.Body)

			if readErr != nil {
				return fiber.NewError(response.StatusCode, "failed reading workbench error")
			}

			return fiber.NewError(response.StatusCode, string(messageBytes))
		}

		stream, err := io.ReadAll(response.Body)

		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		c.Set(fiber.HeaderContentType, arrowStream)

		if err := c.Send(stream); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"hub: send arrow ipc stream",
				err,
			))
		}

		return nil
	})

	return routes
}
