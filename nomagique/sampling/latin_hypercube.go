package sampling

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/stat/distuv"
	"gonum.org/v1/gonum/stat/sampleuv"
)

/*
LatinHypercubeServer draws stratified samples using Latin Hypercube sampling.
*/
type LatinHypercubeServer struct {
	*runtime.System
	samples []float64
}

func NewLatinHypercube(ctx context.Context) *LatinHypercubeServer {
	server := &LatinHypercubeServer{
		System: runtime.NewSystem(ctx, "sampling.latin_hypercube"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and sampling configuration.
*/
func (server *LatinHypercubeServer) Write(ctx context.Context, call LatinHypercube_write) error {
	args := call.Args()
	countVal := int(args.Count())

	if countVal < 0 {
		countVal = 0
	}

	lh := sampleuv.LatinHypercube{Q: distuv.UnitUniform}
	server.samples = make([]float64, countVal)
	lh.Sample(server.samples)
	return nil
}

/*
Done returns calculated sampling results.
*/
func (server *LatinHypercubeServer) Done(ctx context.Context, call LatinHypercube_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[sampling.latin_hypercube.Done] failed to allocate results", err))
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
