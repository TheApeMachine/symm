package calculus

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type ErfcServer struct {
	Downstream types.Float64Sink
}

func (s *ErfcServer) Write(ctx context.Context, call Erfc_write) error {
	a := call.Args().A()
	result := math.Erfc(a)
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *ErfcServer) Done(ctx context.Context, call Erfc_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewErfc() *ErfcServer {
	return &ErfcServer{}
}
