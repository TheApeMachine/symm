package ui

import (
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
)

/*
arrowStream is the media type of an Apache Arrow IPC stream. The dashboard
hands the response body to the Perspective viewer unparsed, so the body is the
stream itself rather than a JSON envelope carrying it.
*/
const arrowStream = "application/vnd.apache.arrow.stream"

/*
registerWorkbench mounts the Analytical Workbench's engine.

This is the only endpoint that evaluates SQL rather than projecting a record
family: the workbench asks questions the typed Hindsight reads were never
shaped to answer, over whatever combination of tables the question needs. The
engine attaches the same Iceberg warehouse those reads use, and answers from
the server so the browser never pulls a table it means to aggregate.

The statements arrive from Perspective running as a Virtual Server: the viewer
compiles its own configuration into SQL and asks only for the window it is
displaying. That is why there is one endpoint and not a REST surface per
question — the viewer, not this package, decides what to ask.

Arbitrary SQL reaches the warehouse from here. The hub listens on loopback by
default and its CORS policy admits loopback origins only; a deployment that
moves it onto a network has to put its own authorization in front of it.
*/
func (hub *Hub) registerWorkbench() {
	// /workbench/query evaluates one statement and returns its result as an
	// Arrow IPC stream, empty for a statement that produces no rows. A
	// malformed or unanswerable statement is the analyst's to see, so the
	// engine's own message is what comes back.
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

		stream, err := hub.warehouse.Execute(hub.ctx, request.SQL)

		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
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
