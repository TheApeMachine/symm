package ui

import (
	"bytes"
	"io"
	"net/http"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	nmcatalog "github.com/theapemachine/symm/nomagique/runtime/catalog"
	"github.com/theapemachine/symm/signal"
)

/*
arrowStream is the media type of an Apache Arrow IPC stream. The dashboard
hands the response body to the Perspective viewer unparsed, so the body is the
stream itself rather than a JSON envelope carrying it.
*/
const arrowStream = "application/vnd.apache.arrow.stream"

/*
registerWorkbench mounts the Analytical Workbench's surface.

Analytical queries are executed out-of-process by the standalone symm-workbench service
to physically isolate DuckDB materializations and Go GC sweeps from the critical trading loop.
The hub acts as a gateway reverse-proxying statements to the workbench service.
*/
func (hub *Hub) registerWorkbench() {
	/*
		/workbench/primitives is the palette the pipeline editor draws from:
		every nomagique primitive, what each is for, and which of its arguments
		are streams to be wired rather than settings to be typed. It is derived
		from nomagique's own declarations, so the editor can only offer what the
		library actually has.
	*/
	hub.app.Get("/workbench/primitives", func(c fiber.Ctx) error {
		primitives, err := nmcatalog.Primitives()

		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.JSON(primitives)
	})

	/*
		/workbench/signals provides access to Flume signal graph definitions.
	*/
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
}
