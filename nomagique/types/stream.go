package types

import (
	"context"
)

/*
StreamNode is the universal interface for dynamically compiled Cap'n Proto nodes.
It encapsulates a native Cap'n Proto streaming server. The type parameters [In, Out]
are used by the AST scanner to generate the UI catalog schema.
*/
type StreamNode[In, Out any] interface {
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

/*
NewStreamNode creates a StreamNode for the dynamic compiler.
*/
func NewStreamNode(
	server any,
	writeAny func(context.Context, any) error,
	setDownstream func(func(context.Context, any) error),
) StreamNode[any, any] {
	return &streamNode{
		server:       server,
		writeAny:     writeAny,
		setDownstream: setDownstream,
	}
}
