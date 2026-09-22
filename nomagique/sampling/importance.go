package sampling

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/stat/distuv"
	"gonum.org/v1/gonum/stat/sampleuv"
)

/*
ImportanceServer performs importance sampling with variance reduction weights.
*/
type ImportanceServer struct {
	*runtime.System
	samples []float64
	weights []float64
}

func NewImportance(ctx context.Context) *ImportanceServer {
	server := &ImportanceServer{
		System: runtime.NewSystem(ctx, "sampling.importance"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and sampling configuration.
*/
func (server *ImportanceServer) Write(ctx context.Context, call Importance_write) error {
	args := call.Args()
	countVal := int(args.Count())
	targetMu := args.TargetMu()
	targetSigma := args.TargetSigma()
	propMu := args.PropMu()
	propSigma := args.PropSigma()

	if countVal < 0 {
		countVal = 0
	}

	if targetSigma <= 0 {
		targetSigma = 1.0
	}

	if propSigma <= 0 {
		propSigma = 1.0
	}

	target := distuv.Normal{Mu: targetMu, Sigma: targetSigma}
	proposal := distuv.Normal{Mu: propMu, Sigma: propSigma}
	imp := sampleuv.Importance{
		Target:   target,
		Proposal: proposal,
	}
	server.samples = make([]float64, countVal)
	server.weights = make([]float64, countVal)
	imp.SampleWeighted(server.samples, server.weights)
	return nil
}

/*
Done returns calculated sampling results.
*/
func (server *ImportanceServer) Done(ctx context.Context, call Importance_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[sampling.importance.Done] failed to allocate results", err))
	}

	listSamples, err := results.NewSamples(int32(len(server.samples)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate samples list", err))
	}

	for index, item := range server.samples {
		listSamples.Set(index, item)
	}

	listWeights, err := results.NewWeights(int32(len(server.weights)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate weights list", err))
	}

	for index, item := range server.weights {
		listWeights.Set(index, item)
	}
	return nil
}
