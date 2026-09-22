package sampling

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/stat/distuv"
	"gonum.org/v1/gonum/stat/sampleuv"
)

/*
IIDServer draws independent and identically distributed uniform random samples.
*/
type IIDServer struct {
	*runtime.System
	samples []float64
}

func NewIID(ctx context.Context) *IIDServer {
	server := &IIDServer{
		System: runtime.NewSystem(ctx, "sampling.iid"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and sampling configuration.
*/
func (server *IIDServer) Write(ctx context.Context, call IID_write) error {
	args := call.Args()
	countVal := int(args.Count())
	minVal := args.Min()
	maxVal := args.Max()

	if countVal < 0 {
		countVal = 0
	}

	dist := distuv.Uniform{Min: minVal, Max: maxVal}
	iid := sampleuv.IIDer{Dist: dist}
	server.samples = make([]float64, countVal)
	iid.Sample(server.samples)
	return nil
}

/*
Done returns calculated sampling results.
*/
func (server *IIDServer) Done(ctx context.Context, call IID_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[sampling.iid.Done] failed to allocate results", err))
	}

	listSamples, err := results.NewSamples(int32(len(server.samples)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate samples list", err))
	}

	for index, item := range server.samples {
		listSamples.Set(index, item)
	}
	return nil
}
