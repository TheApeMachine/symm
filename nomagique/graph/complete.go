package graph

import (
	"context"

	"github.com/theapemachine/errnie"
)

/*
CompleteServer is the complete graph over the nodes present now.
*/
type CompleteServer struct {
	from  []int64
	to    []int64
	pairs []int64
	joint []bool
}

func NewComplete() *CompleteServer {
	return &CompleteServer{}
}

func (server *CompleteServer) Write(ctx context.Context, call Complete_write) error {
	present, err := call.Args().Present()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "graph.complete: failed to read present", err))
	}

	count := int64(present.Len())
	server.from, server.to, server.pairs, server.joint = server.from[:0], server.to[:0], server.pairs[:0], server.joint[:0]

	for from := range count {
		for to := from + 1; to < count; to++ {
			both := present.At(int(from)) && present.At(int(to))
			either := present.At(int(from)) || present.At(int(to))

			if !both && !(either && call.Args().Reach()) {
				continue
			}

			server.from = append(server.from, from)
			server.to = append(server.to, to)
			server.pairs = append(server.pairs, from*count+to)
			server.joint = append(server.joint, both)
		}
	}

	return nil
}

func (server *CompleteServer) Done(ctx context.Context, call Complete_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "graph.complete: failed to allocate results", err))
	}

	from, err := results.NewFromNodes(int32(len(server.from)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "graph.complete: failed to allocate fromNodes", err))
	}

	to, err := results.NewToNodes(int32(len(server.to)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "graph.complete: failed to allocate toNodes", err))
	}

	pairs, err := results.NewPairs(int32(len(server.pairs)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "graph.complete: failed to allocate pairs", err))
	}

	joint, err := results.NewJoint(int32(len(server.joint)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "graph.complete: failed to allocate joint", err))
	}

	for position := range server.from {
		from.Set(position, server.from[position])
		to.Set(position, server.to[position])
		pairs.Set(position, server.pairs[position])
		joint.Set(position, server.joint[position])
	}

	server.from, server.to, server.pairs, server.joint = server.from[:0], server.to[:0], server.pairs[:0], server.joint[:0]
	return nil
}
