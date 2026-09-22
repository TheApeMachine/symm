package optimization

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
ObjectiveServer presents a measured quantity to a minimizer.

A model reports what it is worth and how that changes with its own
parameters. A search descends, and moves in coordinates that are usually not
those parameters. This node closes both gaps: the sense turns a quantity to
be maximized into one to be minimized, and the jacobian of a diagonal
coordinate map carries the gradient into the coordinates the search steps
along.
*/
type ObjectiveServer struct {
	value    float64
	gradient []float64
}

func NewObjective() *ObjectiveServer {
	return &ObjectiveServer{}
}

func (server *ObjectiveServer) Write(ctx context.Context, call Objective_write) error {
	args := call.Args()
	gradient, err := server.read(args.Gradient())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"objective: failed to read gradient",
			err,
		))
	}

	jacobian, err := server.read(args.Jacobian())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"objective: failed to read jacobian",
			err,
		))
	}

	sense := args.Sense()
	server.value = sense * args.Value()
	server.gradient = make([]float64, len(gradient))

	for index, value := range gradient {
		server.gradient[index] = sense * value

		if index < len(jacobian) {
			server.gradient[index] *= jacobian[index]
		}
	}

	return nil
}

/*
read copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *ObjectiveServer) read(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

func (server *ObjectiveServer) Done(ctx context.Context, call Objective_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"objective: failed to allocate results",
			err,
		))
	}

	list, err := results.NewGradient(int32(len(server.gradient)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"objective: failed to allocate gradient list",
			err,
		))
	}

	for index, value := range server.gradient {
		list.Set(index, value)
	}

	results.SetFVal(server.value)
	return nil
}
