package sequence

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type WindowServer struct {
	buf        [][]byte
	size       int
	Downstream func(context.Context, any) error
}

func NewWindowServer(size int) *WindowServer {
	return &WindowServer{size: size, buf: make([][]byte, 0, size)}
}

func (s *WindowServer) Evaluate(ctx context.Context, in any) (any, error) {
	if s.size <= 0 {
		return in, nil
	}

	if s.Downstream != nil {
		return in, s.Downstream(ctx, in)
	}
	return in, nil
}

func (s *WindowServer) Execute(ctx context.Context, call Window_execute) error {
	args, err := call.Args().Window()
	if err != nil {
		return err
	}

	if s.size <= 0 {
		return nil
	}

	payloadPtr, err := args.Payload()
	if err == nil && payloadPtr.IsValid() {
		msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
		if err == nil {
			if msg.SetRoot(payloadPtr) == nil {
				if bytes, err := seg.Message().Marshal(); err == nil {
					if len(s.buf) >= s.size {
						s.buf = s.buf[1:]
					}
					s.buf = append(s.buf, bytes)
				}
			}
		}
	}

	_, err = s.Evaluate(ctx, nil)
	return err
}

type WindowNode types.StreamNode[any, any]

func NewWindowNode(size types.Integer) WindowNode {
	sz := 10
	if size != nil {
		sz = size(nil)
	}
	server := NewWindowServer(sz)
	return types.NewStreamNode(server, func(ctx context.Context, in any) error {
		_, err := server.Evaluate(ctx, in)
		return err
	}, func(next func(context.Context, any) error) {
		server.Downstream = func(ctx context.Context, res any) error {
			return next(ctx, res)
		}
	})
}
