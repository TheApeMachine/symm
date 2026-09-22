package hawkes

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
LogLikelihoodServer evaluates the exact log-likelihood of an observed window
under one parameter set, together with its analytic gradient.

The likelihood of a point process is the sum of the log conditional
intensity at every arrival that was observed, less the integrated intensity
over the whole window. The first term rewards parameters that saw the
arrivals coming; the second charges for the arrivals they expected and did
not get.

The gradient is analytic rather than differenced. The exponential kernel is
accumulated by one forward walk that carries both the excitation and its
derivative with respect to the decay rate, so the whole gradient costs one
pass over the window rather than one pass per parameter.
*/
type LogLikelihoodServer struct {
	value    float64
	gradient []float64
	defined  bool
}

func NewLogLikelihood() *LogLikelihoodServer {
	return &LogLikelihoodServer{}
}

func (server *LogLikelihoodServer) Write(ctx context.Context, call LogLikelihood_write) error {
	args := call.Args()
	times, err := server.readFloats(args.Times())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes log likelihood: failed to read times",
			err,
		))
	}

	components, err := server.readInts(args.Components())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes log likelihood: failed to read components",
			err,
		))
	}

	baseline, err := server.readFloats(args.Baseline())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes log likelihood: failed to read baseline",
			err,
		))
	}

	excitation, err := server.readFloats(args.Excitation())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes log likelihood: failed to read excitation",
			err,
		))
	}

	integral, err := server.readFloats(args.Integral())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes log likelihood: failed to read integral",
			err,
		))
	}

	integralDecay, err := server.readFloats(args.IntegralDecayDerivative())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes log likelihood: failed to read integral decay derivative",
			err,
		))
	}

	server.reset()
	dimension := int(args.Dimension())
	decay := args.Decay()
	origin := args.Origin()
	horizon := args.Horizon()
	span := horizon - origin

	if !server.admissible(dimension, decay, span, baseline, excitation, integral, integralDecay, len(times), len(components)) {
		return nil
	}

	gradient := make([]float64, dimension+dimension*dimension+1)
	value, ok := server.walk(times, components, origin, horizon, baseline, excitation, decay, dimension, gradient)

	if !ok {
		return nil
	}

	server.value = value - server.charge(baseline, excitation, integral, integralDecay, span, decay, dimension, gradient)
	server.gradient = gradient
	server.defined = true
	return nil
}

/*
reset clears the previous evaluation so a refused parameter set never leaves
the last accepted one standing in its place.
*/
func (server *LogLikelihoodServer) reset() {
	server.value = 0
	server.gradient = nil
	server.defined = false
}

/*
admissible reports whether the parameter set and window are shaped such that
a likelihood exists at all.
*/
func (server *LogLikelihoodServer) admissible(
	dimension int,
	decay, span float64,
	baseline, excitation, integral, integralDecay []float64,
	timeCount, componentCount int,
) bool {
	if dimension <= 0 || decay <= 0 || span <= 0 || timeCount == 0 {
		return false
	}

	if timeCount != componentCount {
		return false
	}

	if len(baseline) < dimension || len(excitation) < dimension*dimension {
		return false
	}

	return len(integral) >= dimension && len(integralDecay) >= dimension
}

/*
walk accumulates the log intensity at every observed arrival, carrying the
excitation and its decay derivative forward through the window in one pass.
Arrivals sharing a timestamp are scored against the excitation standing
before any of them landed, so none of them excites another.
*/
func (server *LogLikelihoodServer) walk(
	times []float64,
	components []float64,
	origin, horizon float64,
	baseline, excitation []float64,
	decay float64,
	dimension int,
	gradient []float64,
) (float64, bool) {
	support := make([]float64, dimension)
	derivative := make([]float64, dimension)
	previous := times[0]
	total := 0.0

	for index := 0; index < len(times); {
		at := times[index]

		if at > horizon {
			break
		}

		if at > previous {
			age := at - previous
			factor := math.Exp(-decay * age)

			for component := range support {
				derivative[component] = (derivative[component] - support[component]*age) * factor
				support[component] *= factor
			}

			previous = at
		}

		last := index

		for last < len(times) && times[last] == at {
			last++
		}

		if at > origin && !server.score(components[index:last], baseline, excitation, support, derivative, dimension, gradient, &total) {
			return 0, false
		}

		for _, component := range components[index:last] {
			support[int(component)]++
		}

		index = last
	}

	return total, true
}

/*
score adds the log intensity of every arrival at one instant, and that
instant's contribution to the gradient. A non-positive intensity means the
parameter set claims an arrival that could not have happened, which has no
logarithm and no defined likelihood.
*/
func (server *LogLikelihoodServer) score(
	arriving []float64,
	baseline, excitation, support, derivative []float64,
	dimension int,
	gradient []float64,
	total *float64,
) bool {
	for _, arrival := range arriving {
		component := int(arrival)

		if component < 0 || component >= dimension {
			return false
		}

		row := component * dimension
		intensity := baseline[component]
		decayPart := 0.0

		for column := 0; column < dimension; column++ {
			intensity += excitation[row+column] * support[column]
			decayPart += excitation[row+column] * derivative[column]
		}

		if intensity <= 0 {
			return false
		}

		inverse := 1 / intensity
		*total += math.Log(intensity)
		gradient[component] += inverse

		for column := 0; column < dimension; column++ {
			gradient[dimension+row+column] += inverse * support[column]
		}

		gradient[len(gradient)-1] += inverse * decayPart
	}

	return true
}

/*
charge returns the integrated intensity the likelihood subtracts, and folds
that term's derivatives into the gradient.
*/
func (server *LogLikelihoodServer) charge(
	baseline, excitation, integral, integralDecay []float64,
	span, decay float64,
	dimension int,
	gradient []float64,
) float64 {
	total := 0.0

	for row := 0; row < dimension; row++ {
		total += baseline[row] * span
		gradient[row] -= span

		for column := 0; column < dimension; column++ {
			offset := row*dimension + column
			total += (excitation[offset] / decay) * integral[column]
			gradient[dimension+offset] -= integral[column] / decay
			gradient[len(gradient)-1] -= excitation[offset] *
				(integralDecay[column]/decay - integral[column]/(decay*decay))
		}
	}

	return total
}

/*
readFloats copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *LogLikelihoodServer) readFloats(list capnp.Float64List, err error) ([]float64, error) {
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
readInts copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *LogLikelihoodServer) readInts(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

func (server *LogLikelihoodServer) Done(ctx context.Context, call LogLikelihood_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes log likelihood: failed to allocate results",
			err,
		))
	}

	list, err := results.NewGradient(int32(len(server.gradient)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes log likelihood: failed to allocate gradient list",
			err,
		))
	}

	for index, value := range server.gradient {
		list.Set(index, value)
	}

	results.SetValue(server.value)
	results.SetDefined(server.defined)
	return nil
}
