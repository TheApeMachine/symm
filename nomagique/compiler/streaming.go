package compiler

import (
	"context"
)

/*
StreamNode is the universal interface for dynamically compiled Cap'n Proto nodes.
It allows the JSON graph compiler to wire together arbitrary streaming servers
using reflection at the generic edges, while the servers themselves remain natively typed.
*/
type StreamNode interface {
	WriteAny(ctx context.Context, in any) error
	SetDownstreamAny(func(context.Context, any) error)
}

type streamNode struct {
	server       any
	writeAny     func(context.Context, any) error
	setDownstream func(func(context.Context, any) error)
}

func (s *streamNode) WriteAny(ctx context.Context, in any) error {
	return s.writeAny(ctx, in)
}

func (s *streamNode) SetDownstreamAny(next func(context.Context, any) error) {
	s.setDownstream(next)
}

func NewStreamNode(
	server any,
	writeAny func(context.Context, any) error,
	setDownstream func(func(context.Context, any) error),
) StreamNode {
	return &streamNode{
		server:       server,
		writeAny:     writeAny,
		setDownstream: setDownstream,
	}
}
