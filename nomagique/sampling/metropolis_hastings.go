package sampling

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/stat/distuv"
	"gonum.org/v1/gonum/stat/sampleuv"
)

type GaussianProposal struct {
	Sigma float64
}

func (prop GaussianProposal) ConditionalLogProb(valX, valY float64) float64 {
	norm := distuv.Normal{Mu: valY, Sigma: prop.Sigma}
	return norm.LogProb(valX)
}

func (prop GaussianProposal) ConditionalRand(valY float64) float64 {
	norm := distuv.Normal{Mu: valY, Sigma: prop.Sigma}
	return norm.Rand()
}

/*
MetropolisHastingsServer draws samples using Metropolis-Hastings MCMC algorithm.
*/
type MetropolisHastingsServer struct {
	*runtime.System
	samples []float64
}

func NewMetropolisHastings(ctx context.Context) *MetropolisHastingsServer {
	server := &MetropolisHastingsServer{
		System: runtime.NewSystem(ctx, "sampling.metropolis_hastings"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and sampling configuration.
*/
func (server *MetropolisHastingsServer) Write(ctx context.Context, call MetropolisHastings_write) error {
	args := call.Args()
	countVal := int(args.Count())
	initVal := args.Initial()
	burnIn := int(args.BurnIn())
	rateVal := int(args.Rate())

	if countVal < 0 {
		countVal = 0
	}

	if rateVal <= 0 {
		rateVal = 1
	}

	target := distuv.Normal{Mu: 0, Sigma: 1}
	proposal := GaussianProposal{Sigma: 0.5}
	mh := sampleuv.MetropolisHastings{
		Initial:  initVal,
		Target:   target,
		Proposal: proposal,
		BurnIn:   burnIn,
		Rate:     rateVal,
	}
	server.samples = make([]float64, countVal)
	mh.Sample(server.samples)
	return nil
}

/*
Done returns calculated sampling results.
*/
func (server *MetropolisHastingsServer) Done(ctx context.Context, call MetropolisHastings_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[sampling.metropolis_hastings.Done] failed to allocate results", err))
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
