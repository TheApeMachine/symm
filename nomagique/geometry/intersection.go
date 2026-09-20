package geometry

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type IntersectionServer struct {
	Downstream types.BoolSink
}

func (s *IntersectionServer) Write(ctx context.Context, call Intersection_write) error {
	leftStart := call.Args().LeftStart()
	leftEnd := call.Args().LeftEnd()
	rightStart := call.Args().RightStart()
	rightEnd := call.Args().RightEnd()
	result := leftStart < rightEnd && rightStart < leftEnd
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.BoolSink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *IntersectionServer) Done(ctx context.Context, call Intersection_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewIntersection() *IntersectionServer {
	return &IntersectionServer{}
}
