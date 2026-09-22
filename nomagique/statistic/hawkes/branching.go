package hawkes

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
BranchingServer forms the branching matrix of a multivariate Hawkes process.
Dividing the excitation matrix by the decay rate integrates each exponential
kernel over all future time, so entry (k, j) becomes the expected number of
first-generation component-k arrivals one component-j arrival triggers.
*/
type BranchingServer struct {
	branching []float64
}

func NewBranching() *BranchingServer {
	return &BranchingServer{}
}

func (server *BranchingServer) Write(ctx context.Context, call Branching_write) error {
	args := call.Args()
	excitation, err := server.read(args.Excitation())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes branching: failed to read excitation",
			err,
		))
	}

	decay := args.Decay()
	dimension := int(args.Dimension())

	if decay <= 0 || dimension <= 0 || len(excitation) < dimension*dimension {
		server.branching = nil
		return nil
	}

	branching := make([]float64, dimension*dimension)

	for index := range branching {
		branching[index] = excitation[index] / decay
	}

	server.branching = branching
	return nil
}

/*
read copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *BranchingServer) read(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

func (server *BranchingServer) Done(ctx context.Context, call Branching_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes branching: failed to allocate results",
			err,
		))
	}

	list, err := results.NewBranching(int32(len(server.branching)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes branching: failed to allocate branching list",
			err,
		))
	}

	for index, value := range server.branching {
		list.Set(index, value)
	}

	return nil
}
