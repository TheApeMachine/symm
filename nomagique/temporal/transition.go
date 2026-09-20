package temporal

import (
	"context"
)

type TransitionServer struct {
	Downstream func(context.Context, []byte) error
	previous   []byte
}

func (s *TransitionServer) Write(ctx context.Context, call Transition_write) error {
	a, _ := call.Args().A()
	var result []byte
	if len(a) > 0 {
		if len(s.previous) == 0 {
			s.previous = make([]byte, len(a))
			copy(s.previous, a)
		} else {
			result = make([]byte, len(s.previous)+2+len(a))
			copy(result, s.previous)
			result[len(s.previous)] = '-'
			result[len(s.previous)+1] = '>'
			copy(result[len(s.previous)+2:], a)
			s.previous = make([]byte, len(a))
			copy(s.previous, a)
		}
	}
	return s.Downstream(ctx, result)
}

func (s *TransitionServer) Done(ctx context.Context, call Transition_done) error {
	return nil
}
