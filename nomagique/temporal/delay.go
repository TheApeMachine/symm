package temporal

import (
	"context"
	"errors"
	
	"github.com/theapemachine/symm/nomagique/types"
)

type DelayServer struct {
	Downstream func(context.Context, float64) error
	horizon    int
	buffer     []float64
	idx        int
}

func (s *DelayServer) Write(ctx context.Context, call Delay_write) error {
	val := call.Args().A()
	
	if len(s.buffer) < s.horizon {
		s.buffer = append(s.buffer, val)
		return nil
	}

	delayed := s.buffer[s.idx]
	s.buffer[s.idx] = val
	s.idx = (s.idx + 1) % s.horizon

	return s.Downstream(ctx, delayed)
}

func (s *DelayServer) Done(ctx context.Context, call Delay_done) error {
	return nil
}

type DelayNode types.StreamNode[any, any]

func NewDelay(horizon types.Integer) (DelayNode, error) {
	h := horizon(nil)
	if h <= 0 {
		return nil, errors.New("requires positive 'horizon'")
	}
	
	server := &DelayServer{horizon: h}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {
			server.Downstream = func(c context.Context, p float64) error {
				return next(c, p)
			}
		},
	), nil
}
