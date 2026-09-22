package hawkes

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
CompensatorServer evaluates the integrated conditional intensity of each
component over the observation window: how many arrivals the fitted process
expected to see. Comparing it against how many actually arrived is the
process's own residual, and it is the term the log-likelihood subtracts.
*/
type CompensatorServer struct {
	compensator []float64
}

func NewCompensator() *CompensatorServer {
	return &CompensatorServer{}
}

func (server *CompensatorServer) Write(ctx context.Context, call Compensator_write) error {
	args := call.Args()
	baseline, err := server.read(args.Baseline())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes compensator: failed to read baseline",
			err,
		))
	}

	excitation, err := server.read(args.Excitation())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes compensator: failed to read excitation",
			err,
		))
	}

	support, err := server.read(args.Support())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes compensator: failed to read support",
			err,
		))
	}

	span := args.Span()
	decay := args.Decay()
	dimension := int(args.Dimension())
	server.compensator = nil

	if decay <= 0 || span <= 0 || dimension <= 0 {
		return nil
	}

	if len(baseline) < dimension || len(support) < dimension || len(excitation) < dimension*dimension {
		return nil
	}

	compensator := make([]float64, dimension)

	for row := 0; row < dimension; row++ {
		total := baseline[row] * span

		for column := 0; column < dimension; column++ {
			total += (excitation[row*dimension+column] / decay) * support[column]
		}

		compensator[row] = total
	}

	server.compensator = compensator
	return nil
}

/*
read copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *CompensatorServer) read(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

func (server *CompensatorServer) Done(ctx context.Context, call Compensator_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes compensator: failed to allocate results",
			err,
		))
	}

	list, err := results.NewCompensator(int32(len(server.compensator)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes compensator: failed to allocate compensator list",
			err,
		))
	}

	for index, value := range server.compensator {
		list.Set(index, value)
	}

	return nil
}
