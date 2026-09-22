package sampling

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/stat/sampleuv"
)

/*
WeightedServer samples without replacement from non-uniform categorical weights.
*/
type WeightedServer struct {
	*runtime.System
	indices []int64
}

func NewWeighted(ctx context.Context) *WeightedServer {
	server := &WeightedServer{
		System: runtime.NewSystem(ctx, "sampling.weighted"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and sampling configuration.
*/
func (server *WeightedServer) Write(ctx context.Context, call Weighted_write) error {
	args := call.Args()
	weightList, err := args.Weights()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read weights", err))
	}

	countVal := int(args.Count())
	weights := make([]float64, weightList.Len())

	for index := 0; index < weightList.Len(); index++ {
		weights[index] = weightList.At(index)
	}

	weighted := sampleuv.NewWeighted(weights, nil)
	server.indices = make([]int64, 0, countVal)

	for stepIdx := 0; stepIdx < countVal; stepIdx++ {
		chosen, ok := weighted.Take()

		if !ok {
			break
		}

		server.indices = append(server.indices, int64(chosen))
	}
	return nil
}

/*
Done returns calculated sampling results.
*/
func (server *WeightedServer) Done(ctx context.Context, call Weighted_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[sampling.weighted.Done] failed to allocate results", err))
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
