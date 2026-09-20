package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type RouteServer struct {
	out []byte
}

func NewRoute() *RouteServer {
	return &RouteServer{}
}

func (server *RouteServer) Write(ctx context.Context, call Route_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *RouteServer) Done(ctx context.Context, call Route_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"route: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"route: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
