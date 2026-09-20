package transport

import (
	"context"
)

type RouteServer struct {
	Downstream func(context.Context, any) error
}

func NewRoute() *RouteServer {
	return &RouteServer{}
}

func (s *RouteServer) Write(ctx context.Context, call Route_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *RouteServer) Done(ctx context.Context, call Route_done) error {
	return nil
}
