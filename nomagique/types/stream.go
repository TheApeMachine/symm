package types

import (
	"context"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/rpc"
	"github.com/theapemachine/symm/pkg/network"
)

type StreamNode[In, Out any] interface {
	Serve(ctx context.Context, listen network.ManagedTransport) error
	AddDownstream(ctx context.Context, port string, client capnp.Client) error
	
	// Temporary shims to keep primitive signatures intact during the migration script.
	WriteAny(ctx context.Context, in any) error
	SetDownstreamAny(func(context.Context, any) error)
}

type streamNode struct {
	client        capnp.Client
	addDownstream func(context.Context, string, capnp.Client) error
	
	writeAny     func(context.Context, any) error
	setDownstream func(func(context.Context, any) error)
}

func (s *streamNode) Serve(ctx context.Context, listen network.ManagedTransport) error {
	if ready, ok := listen.(network.ReadyTransport); ok {
		if err := ready.Ready(ctx); err != nil {
			return err
		}
	}
	conn := rpc.NewConn(rpc.NewStreamTransport(listen), &rpc.Options{
		BootstrapClient: s.client,
	})
	<-conn.Done()
	return nil
}

func (s *streamNode) AddDownstream(ctx context.Context, port string, client capnp.Client) error {
	if s.addDownstream != nil {
		return s.addDownstream(ctx, port, client)
	}
	return nil
}

func (s *streamNode) WriteAny(ctx context.Context, in any) error {
	if s.writeAny != nil {
		return s.writeAny(ctx, in)
	}
	return nil
}

func (s *streamNode) SetDownstreamAny(next func(context.Context, any) error) {
	if s.setDownstream != nil {
		s.setDownstream(next)
	}
}

// Temporary shim to keep primitive signatures intact during the migration script.
func NewStreamNode(
	server any,
	writeAny func(context.Context, any) error,
	setDownstream func(func(context.Context, any) error),
) StreamNode[any, any] {
	return &streamNode{
		writeAny: writeAny,
		setDownstream: setDownstream,
	}
}
