package sequence

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
)

type WindowServer struct {
	buf        [][]byte
	size       int
	Downstream func(context.Context, any) error
}

func NewWindow() *WindowServer {
	return &WindowServer{size: 10, buf: make([][]byte, 0, 10)}
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
