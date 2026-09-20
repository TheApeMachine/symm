package geometry

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type IntersectionServer struct {
	DownstreamIntersection func(context.Context, bool) error
}

func (s *IntersectionServer) Write(ctx context.Context, call Intersection_write) error {
	leftStart := call.Args().LeftStart()
	leftEnd := call.Args().LeftEnd()
	rightStart := call.Args().RightStart()
	rightEnd := call.Args().RightEnd()
	result := leftStart < rightEnd && rightStart < leftEnd
	if s.DownstreamIntersection != nil {
		return s.DownstreamIntersection(ctx, result)
	}
	return nil
}

func (s *IntersectionServer) Done(ctx context.Context, call Intersection_done) error {
	return nil
}



type IntersectionNode types.StreamNode[any, any]

func NewIntersection() IntersectionNode {
	server := &IntersectionServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
