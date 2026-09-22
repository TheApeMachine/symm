package hawkes

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
softplusLinear is where softplus stops differing from its argument in double
precision: above it exp(x) overflows the useful range while log1p(exp(x))
has already converged on x, so evaluating the closed form buys nothing and
risks an infinity.
*/
const softplusLinear = 20

/*
ParametersServer maps a point in an optimizer's unconstrained coordinates
onto the parameters of the process.

Every parameter of a Hawkes process is strictly positive and bounded by what
the observation can support, but optimizers search the whole real line. Each
coordinate is therefore squashed into its own bound by a softplus ratio and
exponentiated out of log space, so no search step can propose a negative
rate or a decay outside the range the data resolves.

The map is diagonal, so it also reports its own derivative: a gradient
measured in natural parameters is carried back into coordinates by
multiplying it entry by entry with the jacobian.
*/
type ParametersServer struct {
	baseline   []float64
	excitation []float64
	decay      float64
	jacobian   []float64
}

func NewParameters() *ParametersServer {
	return &ParametersServer{}
}

func (server *ParametersServer) Write(ctx context.Context, call Parameters_write) error {
	args := call.Args()
	coordinates, err := server.read(args.Coordinates())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes parameters: failed to read coordinates",
			err,
		))
	}

	lower, err := server.read(args.Lower())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes parameters: failed to read lower bounds",
			err,
		))
	}

	upper, err := server.read(args.Upper())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes parameters: failed to read upper bounds",
			err,
		))
	}

	dimension := int(args.Dimension())
	server.clear()

	if dimension <= 0 {
		return nil
	}

	width := dimension + dimension*dimension + 1

	if len(coordinates) < width || len(lower) < width || len(upper) < width {
		return nil
	}

	values := make([]float64, width)
	jacobian := make([]float64, width)

	for index := 0; index < width; index++ {
		values[index], jacobian[index] = server.decode(coordinates[index], lower[index], upper[index])
	}

	server.baseline = values[:dimension]
	server.excitation = values[dimension : dimension+dimension*dimension]
	server.decay = values[width-1]
	server.jacobian = jacobian
	return nil
}

/*
clear drops the previous mapping so a refused coordinate set never leaves the
last accepted parameters standing in its place.
*/
func (server *ParametersServer) clear() {
	server.baseline = nil
	server.excitation = nil
	server.decay = 0
	server.jacobian = nil
}

/*
decode squashes one coordinate into its bound and leaves log space, and
returns the derivative of that value with respect to the coordinate.

The squash is the softplus ratio s/(1+s): it is strictly increasing, it
approaches the bounds without ever reaching them, and it is smooth
everywhere, so a search never lands exactly on a boundary and the gradient
never has a corner in it.
*/
func (server *ParametersServer) decode(coordinate, lower, upper float64) (float64, float64) {
	span := upper - lower

	if span <= 0 {
		return math.Exp(lower), 0
	}

	lift := server.softplus(coordinate)
	ratio := lift / (1 + lift)
	value := math.Exp(lower + span*ratio)
	slope := server.logistic(coordinate) / ((1 + lift) * (1 + lift))

	return value, value * span * slope
}

/*
softplus returns log(1 + exp(value)), falling back to the identity where the
two agree to double precision.
*/
func (server *ParametersServer) softplus(value float64) float64 {
	if value > softplusLinear {
		return value
	}

	return math.Log1p(math.Exp(value))
}

/*
logistic returns the derivative of softplus.
*/
func (server *ParametersServer) logistic(value float64) float64 {
	if value > softplusLinear {
		return 1
	}

	if value < -softplusLinear {
		return math.Exp(value)
	}

	return 1 / (1 + math.Exp(-value))
}

/*
read copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *ParametersServer) read(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

func (server *ParametersServer) Done(ctx context.Context, call Parameters_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes parameters: failed to allocate results",
			err,
		))
	}

	baselineList, err := results.NewBaseline(int32(len(server.baseline)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes parameters: failed to allocate baseline list",
			err,
		))
	}

	for index, value := range server.baseline {
		baselineList.Set(index, value)
	}

	excitationList, err := results.NewExcitation(int32(len(server.excitation)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes parameters: failed to allocate excitation list",
			err,
		))
	}

	for index, value := range server.excitation {
		excitationList.Set(index, value)
	}

	jacobianList, err := results.NewJacobian(int32(len(server.jacobian)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes parameters: failed to allocate jacobian list",
			err,
		))
	}

	for index, value := range server.jacobian {
		jacobianList.Set(index, value)
	}

	results.SetDecay(server.decay)
	return nil
}
