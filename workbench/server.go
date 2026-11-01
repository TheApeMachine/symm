package workbench

import (
	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
)

const arrowStream = "application/vnd.apache.arrow.stream"

/*
Server serves POST /workbench/query over the warehouse.
*/
type Server struct {
	warehouse *Warehouse
	app       *fiber.App
}

/*
NewServer constructs the Analytical Workbench HTTP surface.
*/
func NewServer(warehouse *Warehouse) *Server {
	server := &Server{
		warehouse: warehouse,
		app: fiber.New(fiber.Config{
			JSONEncoder: sonic.Marshal,
			JSONDecoder: sonic.Unmarshal,
		}),
	}

	server.app.Post("/workbench/query", server.query)

	return server
}

/*
Listen binds the configured workbench address.
*/
func (server *Server) Listen() error {
	return server.app.Listen(system.Cfg.Workbench.Addr)
}

func (server *Server) query(c fiber.Ctx) error {
	var request struct {
		SQL string `json:"sql"`
	}

	if err := c.Bind().Body(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	if request.SQL == "" {
		return fiber.NewError(fiber.StatusBadRequest, "query is empty")
	}

	stream, err := server.warehouse.Execute(c.Context(), request.SQL)

	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	c.Set(fiber.HeaderContentType, arrowStream)

	if err := c.Send(stream); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO, "workbench: send arrow ipc stream", err,
		))
	}

	return nil
}
