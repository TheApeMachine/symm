package sampling

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/stat/sampleuv"
)

/*
WithoutReplacementServer samples integers without replacement from uniform population [0, n).
*/
type WithoutReplacementServer struct {
	*runtime.System
	indices []int64
}

func NewWithoutReplacement(ctx context.Context) *WithoutReplacementServer {
	server := &WithoutReplacementServer{
		System: runtime.NewSystem(ctx, "sampling.without_replacement"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and sampling configuration.
*/
func (server *WithoutReplacementServer) Write(ctx context.Context, call WithoutReplacement_write) error {
	args := call.Args()
	countVal := int(args.Count())
	limitN := int(args.N())

	if countVal < 0 {
		countVal = 0
	}

	if countVal > limitN {
		countVal = limitN
	}

	rawIndices := make([]int, countVal)
	sampleuv.WithoutReplacement(rawIndices, limitN, nil)
	server.indices = make([]int64, countVal)

	for index, item := range rawIndices {
		server.indices[index] = int64(item)
	}
	return nil
}

/*
Done returns calculated sampling results.
*/
func (server *WithoutReplacementServer) Done(ctx context.Context, call WithoutReplacement_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[sampling.without_replacement.Done] failed to allocate results", err))
	}

	listIndices, err := results.NewIndices(int32(len(server.indices)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate indices list", err))
	}

	for index, item := range server.indices {
		listIndices.Set(index, item)
	}
	return nil
}
