package data

import (
	"context"
	"errors"

	"github.com/theapemachine/symm/nomagique/types"
)

type ExtractServer struct {
	Downstream func(context.Context, float64) error
}

func NewExtractServer() *ExtractServer {
	return &ExtractServer{}
}

func (s *ExtractServer) Evaluate(ctx context.Context, payload any) (float64, error) {
	val := 0.0
	if v, ok := payload.(float64); ok {
		val = v
	}

	if s.Downstream != nil {
		return val, s.Downstream(ctx, val)
	}

	return val, nil
}

func (s *ExtractServer) Write(ctx context.Context, call Extract_write) error {
	args, err := call.Args().Extract()
	if err != nil {
		return err
	}

	payloadPtr, err := args.Payload()
	if err != nil {
		return err
	}

	var payload any
	if payloadPtr.IsValid() {
		// placeholder
	}

	_, evalErr := s.Evaluate(ctx, payload)
	return evalErr
}

func (s *ExtractServer) Done(ctx context.Context, call Extract_done) error {
	return nil
}

type ExtractNode types.StreamNode[any, float64]

func NewExtract(path types.String) (ExtractNode, error) {
	p := path(nil)
	if p == "" {
		return nil, errors.New("requires 'path' or 'key' configuration")
	}
	
	server := &ExtractServer{}
	return types.NewStreamNode(server, func(ctx context.Context, in any) error {
		_, err := server.Evaluate(ctx, in)
		return err
	}, func(next func(context.Context, any) error) {
		server.Downstream = func(ctx context.Context, res float64) error {
			return next(ctx, res)
		}
	}), nil
}
