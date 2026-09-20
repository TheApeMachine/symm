package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type HTTPRequestServer struct {
	out []byte
}

func NewHTTPRequest() *HTTPRequestServer {
	return &HTTPRequestServer{}
}

func (server *HTTPRequestServer) Write(ctx context.Context, call HTTPRequest_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *HTTPRequestServer) Done(ctx context.Context, call HTTPRequest_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"httprequest: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"httprequest: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
