package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type RouteServer struct {
	Downstream func(context.Context, any) error
}

func NewRouteServer() *RouteServer {
	return &RouteServer{}
}

func (s *RouteServer) Write(ctx context.Context, call Route_write) error {
	if s.Downstream != nil {
		// Placeholder for Route processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *RouteServer) Done(ctx context.Context, call Route_done) error {
	return nil
}



type RouteNode types.StreamNode[any, any]

func NewRoute() RouteNode {
	server := &RouteServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
