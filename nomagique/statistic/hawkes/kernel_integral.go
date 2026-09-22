package hawkes

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
KernelIntegralServer evaluates the exponential kernel integrated over the
observation window for each component, together with its derivative with
respect to the decay rate.

The exponential kernel integrates in closed form, so no quadrature is
involved: over an arrival's age range the integral is the difference of two
exponentials. Arrivals before the window's origin still contribute, but only
over the part of their decay that falls inside the window.
*/
type KernelIntegralServer struct {
	support         []float64
	decayDerivative []float64
}

func NewKernelIntegral() *KernelIntegralServer {
	return &KernelIntegralServer{}
}

func (server *KernelIntegralServer) Write(ctx context.Context, call KernelIntegral_write) error {
	args := call.Args()
	times, err := server.readTimes(args.Times())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes kernel integral: failed to read times",
			err,
		))
	}

	components, err := server.readComponents(args.Components())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes kernel integral: failed to read components",
			err,
		))
	}

	origin := args.Origin()
	horizon := args.Horizon()
	decay := args.Decay()
	dimension := int(args.Dimension())

	if decay <= 0 || dimension <= 0 {
		server.support = nil
		server.decayDerivative = nil
		return nil
	}

	support := make([]float64, dimension)
	derivative := make([]float64, dimension)

	for index, eventTime := range times {
		if eventTime > horizon || index >= len(components) {
			continue
		}

		component := int(components[index])

		if component < 0 || component >= dimension {
			continue
		}

		lowerAge := math.Max(0, origin-eventTime)
		upperAge := horizon - eventTime

		if upperAge <= lowerAge {
			continue
		}

		lower := math.Exp(-decay * lowerAge)
		upper := math.Exp(-decay * upperAge)
		support[component] += lower - upper
		derivative[component] += upperAge*upper - lowerAge*lower
	}

	server.support = support
	server.decayDerivative = derivative
	return nil
}

/*
readTimes copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *KernelIntegralServer) readTimes(list capnp.Float64List, err error) ([]float64, error) {
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
func (server *KernelIntegralServer) readComponents(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

func (server *KernelIntegralServer) Done(ctx context.Context, call KernelIntegral_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes kernel integral: failed to allocate results",
			err,
		))
	}

	supportList, err := results.NewSupport(int32(len(server.support)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes kernel integral: failed to allocate support list",
			err,
		))
	}

	for index, value := range server.support {
		supportList.Set(index, value)
	}

	derivativeList, err := results.NewDecayDerivative(int32(len(server.decayDerivative)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes kernel integral: failed to allocate decay derivative list",
			err,
		))
	}

	for index, value := range server.decayDerivative {
		derivativeList.Set(index, value)
	}

	return nil
}
