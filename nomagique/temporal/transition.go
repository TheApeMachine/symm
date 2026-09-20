package temporal

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type TransitionServer struct {
	Downstream types.DataSink
	previous   []byte
}

func (s *TransitionServer) Write(ctx context.Context, call Transition_write) error {
	a, _ := call.Args().A()
	var result []byte
	if len(a) > 0 {
		if len(s.previous) == 0 {
			s.previous = make([]byte, len(a))
			copy(s.previous, a)
		}

		if len(s.previous) > 0 && (len(s.previous) != len(a) || string(s.previous) != string(a)) {
			result = make([]byte, len(s.previous)+2+len(a))
			copy(result, s.previous)
			result[len(s.previous)] = '-'
			result[len(s.previous)+1] = '>'
			copy(result[len(s.previous)+2:], a)
			s.previous = make([]byte, len(a))
			copy(s.previous, a)
		}
	}

	if len(result) > 0 && capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.DataSink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *TransitionServer) Done(ctx context.Context, call Transition_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewTransition() *TransitionServer {
	return &TransitionServer{}
}
