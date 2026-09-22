package sampling

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/stat/distuv"
	"gonum.org/v1/gonum/stat/sampleuv"
)

/*
RejectionServer draws samples using rejection sampling algorithm.
*/
type RejectionServer struct {
	*runtime.System
	samples  []float64
	proposed int32
}

func NewRejection(ctx context.Context) *RejectionServer {
	server := &RejectionServer{
		System: runtime.NewSystem(ctx, "sampling.rejection"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and sampling configuration.
*/
func (server *RejectionServer) Write(ctx context.Context, call Rejection_write) error {
	args := call.Args()
	countVal := int(args.Count())
	constC := args.C()
	targetScale := args.TargetScale()
	proposalScale := args.ProposalScale()

	if countVal < 0 {
		countVal = 0
	}

	if targetScale <= 0 {
		targetScale = 1.0
	}

	if proposalScale <= 0 {
		proposalScale = 1.0
	}

	if constC <= 0 {
		constC = 1.0
	}

	target := distuv.Laplace{Mu: 0, Scale: targetScale}
	proposal := distuv.Normal{Mu: 0, Sigma: proposalScale}
	rej := &sampleuv.Rejection{
		Target:   target,
		Proposal: proposal,
		C:        constC,
	}
	server.samples = make([]float64, countVal)
	rej.Sample(server.samples)
	server.proposed = int32(rej.Proposed())
	return nil
}

/*
Done returns calculated sampling results.
*/
func (server *RejectionServer) Done(ctx context.Context, call Rejection_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[sampling.rejection.Done] failed to allocate results", err))
	}

	listSamples, err := results.NewSamples(int32(len(server.samples)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate samples list", err))
	}

	for index, item := range server.samples {
		listSamples.Set(index, item)
	}
	results.SetProposed(server.proposed)
	return nil
}
