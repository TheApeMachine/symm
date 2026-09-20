package transport

import (
	"context"
)

type HTTPRequestServer struct {
	Downstream func(context.Context, any) error
}

func NewHTTPRequestServer() *HTTPRequestServer {
	return &HTTPRequestServer{}
}

func (s *HTTPRequestServer) Write(ctx context.Context, call HTTPRequest_write) error {
	if s.Downstream != nil {
		// Placeholder for HTTPRequest processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *HTTPRequestServer) Done(ctx context.Context, call HTTPRequest_done) error {
	return nil
}
