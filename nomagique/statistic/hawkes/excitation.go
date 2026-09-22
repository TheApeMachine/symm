package hawkes

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
ExcitationServer evaluates the exponential kernel's support at the horizon:
for each component, the sum of exp(-decay * age) over that component's
arrivals, where age is how long ago the arrival happened.

An arrival exactly at the horizon contributes nothing. The conditional
intensity has to be predictable, meaning it is determined by what happened
strictly before the instant it describes; letting an arrival excite its own
instant would make the likelihood count it twice.
*/
type ExcitationServer struct {
	support []float64
}

func NewExcitation() *ExcitationServer {
	return &ExcitationServer{}
}

func (server *ExcitationServer) Write(ctx context.Context, call Excitation_write) error {
	args := call.Args()
	times, err := server.readTimes(args.Times())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes excitation: failed to read times",
			err,
		))
	}

	components, err := server.readComponents(args.Components())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes excitation: failed to read components",
			err,
		))
	}

	horizon := args.Horizon()
	decay := args.Decay()
	dimension := int(args.Dimension())

	if decay <= 0 || dimension <= 0 {
		server.support = nil
		return nil
	}

	support := make([]float64, dimension)

	for index, eventTime := range times {
		age := horizon - eventTime

		if age <= 0 || index >= len(components) {
			continue
		}

		component := int(components[index])

		if component < 0 || component >= dimension {
			continue
		}

		support[component] += math.Exp(-decay * age)
	}

	server.support = support
	return nil
}

/*
readTimes copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *ExcitationServer) readTimes(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

/*
readComponents copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *ExcitationServer) readComponents(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

func (server *ExcitationServer) Done(ctx context.Context, call Excitation_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes excitation: failed to allocate results",
			err,
		))
	}

	list, err := results.NewSupport(int32(len(server.support)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes excitation: failed to allocate support list",
			err,
		))
	}

	for index, value := range server.support {
		list.Set(index, value)
	}

	return nil
}
