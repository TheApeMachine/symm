package ui

import (
	"context"

	"github.com/theapemachine/errnie"
)

type UIRouteServer struct {
	path  string
	title string
}

func NewUIRoute() *UIRouteServer {
	return &UIRouteServer{}
}

func (server *UIRouteServer) Write(ctx context.Context, call UIRoute_write) error {
	path, err := call.Args().Path()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"ui: read path failed",
			err,
		))
	}

	title, err := call.Args().Title()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"ui: read title failed",
			err,
		))
	}

	server.path = path
	server.title = title
	return nil
}

func (server *UIRouteServer) Done(ctx context.Context, call UIRoute_done) error {
	return nil
}
