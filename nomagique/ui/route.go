package ui

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
UIRouteServer holds one authored route for the duration of a single evaluation.
A route names a path and points at the component subgraph that renders it.
*/
type UIRouteServer struct {
	route Route
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

	components, err := call.Args().Components()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"ui: read route components failed",
			err,
		))
	}

	route, err := newDetachedRoute()
	if err != nil {
		return err
	}

	if err := route.SetPath(path); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: set path failed",
			err,
		))
	}

	if err := route.SetTitle(title); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: set title failed",
			err,
		))
	}

	if components.IsValid() {
		if err := route.SetComponents(components); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"ui: set route components failed",
				err,
			))
		}
	}

	server.route = route
	return nil
}

func (server *UIRouteServer) Done(ctx context.Context, call UIRoute_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: alloc route results failed",
			err,
		))
	}

	if err := results.SetOut(server.route); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: set route result failed",
			err,
		))
	}

	server.route = Route{}
	return nil
}

/*
newDetachedRoute allocates a route in its own message so the value outlives the
incoming call arguments, which Cap'n Proto releases once Write returns.
*/
func newDetachedRoute() (Route, error) {
	_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return Route{}, errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: new route message failed",
			err,
		))
	}

	route, err := NewRootRoute(segment)
	if err != nil {
		return Route{}, errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: new root route failed",
			err,
		))
	}

	return route, nil
}
