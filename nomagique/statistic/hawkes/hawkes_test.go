package hawkes_test

import (
	capnp "capnproto.org/go/capnp/v3"
	"context"
	"math"
	"math/rand"
	"testing"

	"github.com/theapemachine/symm/nomagique/optimization"
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

/*
realisation is one simulated path of a multivariate Hawkes process: the
arrival times and the component each landed on, in the order they happened.
It is the single fixture every test in this package measures nodes against.
*/
type realisation struct {
	times      []float64
	components []float64
	dimension  int
	origin     float64
	horizon    float64
}

/*
simulate draws a path by Ogata's thinning algorithm: propose arrivals from a
rate that dominates the true intensity, then keep each with the probability
that the true intensity is of the dominating one. The result is an exact
sample of the process, not an approximation of it, which is what lets the
tests state what the estimator should recover.
*/
func simulate(
	dimension int,
	baseline, excitation []float64,
	decay, horizon float64,
	seed int64,
) realisation {
	source := rand.New(rand.NewSource(seed))
	path := realisation{dimension: dimension, horizon: horizon}
	support := make([]float64, dimension)
	current := 0.0
	previous := 0.0

	for current < horizon {
		bound := 0.0

		for row := 0; row < dimension; row++ {
			rate := baseline[row]

			for column := 0; column < dimension; column++ {
				rate += excitation[row*dimension+column] * support[column]
			}

			bound += rate
		}

		if bound <= 0 {
			break
		}

		current += source.ExpFloat64() / bound

		if current >= horizon {
			break
		}

		factor := math.Exp(-decay * (current - previous))

		for index := range support {
			support[index] *= factor
		}

		previous = current
		intensity := make([]float64, dimension)
		total := 0.0

		for row := 0; row < dimension; row++ {
			rate := baseline[row]

			for column := 0; column < dimension; column++ {
				rate += excitation[row*dimension+column] * support[column]
			}

			intensity[row] = rate
			total += rate
		}

		if source.Float64()*bound > total {
			continue
		}

		draw := source.Float64() * total
		chosen := dimension - 1

		for row := 0; row < dimension; row++ {
			draw -= intensity[row]

			if draw <= 0 {
				chosen = row
				break
			}
		}

		path.times = append(path.times, current)
		path.components = append(path.components, float64(chosen))
		support[chosen]++
	}

	if len(path.times) > 0 {
		path.origin = path.times[0]
		path.horizon = path.times[len(path.times)-1]
	}

	return path
}

/*
integrate drives the KernelIntegral node over a path, returning the
integrated kernel support per component and its decay derivative.
*/
func integrate(t *testing.T, path realisation, decay float64) ([]float64, []float64) {
	t.Helper()
	ctx := context.Background()
	client := hawkes.KernelIntegral_ServerToClient(hawkes.NewKernelIntegral())

	err := client.Write(ctx, func(params hawkes.KernelIntegral_write_Params) error {
		if err := writeFloats(params.NewTimes, path.times); err != nil {
			return err
		}

		if err := writeFloats(params.NewComponents, path.components); err != nil {
			return err
		}

		params.SetOrigin(path.origin)
		params.SetHorizon(path.horizon)
		params.SetDecay(decay)
		params.SetDimension(int32(path.dimension))
		return nil
	})

	if err != nil {
		t.Fatalf("kernel integral write: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("kernel integral stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("kernel integral done: %v", err)
	}

	support, err := results.Support()

	if err != nil {
		t.Fatalf("kernel integral support: %v", err)
	}

	derivative, err := results.DecayDerivative()

	if err != nil {
		t.Fatalf("kernel integral derivative: %v", err)
	}

	return readList(support.Len(), support.At), readList(derivative.Len(), derivative.At)
}

/*
likelihood drives the composition a fit actually runs: KernelIntegral feeding
LogLikelihood. It returns the log-likelihood, its analytic gradient, and
whether the parameter set admits one at all.
*/
func likelihood(
	t *testing.T,
	path realisation,
	baseline, excitation []float64,
	decay float64,
) (float64, []float64, bool) {
	t.Helper()
	integral, integralDecay := integrate(t, path, decay)
	ctx := context.Background()
	client := hawkes.LogLikelihood_ServerToClient(hawkes.NewLogLikelihood())

	err := client.Write(ctx, func(params hawkes.LogLikelihood_write_Params) error {
		if err := writeFloats(params.NewTimes, path.times); err != nil {
			return err
		}

		if err := writeFloats(params.NewComponents, path.components); err != nil {
			return err
		}

		if err := writeFloats(params.NewBaseline, baseline); err != nil {
			return err
		}

		if err := writeFloats(params.NewExcitation, excitation); err != nil {
			return err
		}

		if err := writeFloats(params.NewIntegral, integral); err != nil {
			return err
		}

		if err := writeFloats(params.NewIntegralDecayDerivative, integralDecay); err != nil {
			return err
		}

		params.SetOrigin(path.origin)
		params.SetHorizon(path.horizon)
		params.SetDecay(decay)
		params.SetDimension(int32(path.dimension))
		return nil
	})

	if err != nil {
		t.Fatalf("log likelihood write: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("log likelihood stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("log likelihood done: %v", err)
	}

	gradient, err := results.Gradient()

	if err != nil {
		t.Fatalf("log likelihood gradient: %v", err)
	}

	return results.Value(), readList(gradient.Len(), gradient.At), results.Defined()
}

/*
writeFloats fills a Cap'n Proto float list allocated by the caller's builder.
*/
func writeFloats(allocate func(int32) (capnp.Float64List, error), values []float64) error {
	list, err := allocate(int32(len(values)))

	if err != nil {
		return err
	}

	for index, value := range values {
		list.Set(index, value)
	}

	return nil
}

/*
writeInts fills a Cap'n Proto integer list allocated by the caller's builder.
*/
func writeInts(allocate func(int32) (capnp.Int32List, error), values []int32) error {
	list, err := allocate(int32(len(values)))

	if err != nil {
		return err
	}

	for index, value := range values {
		list.Set(index, value)
	}

	return nil
}

/*
readList copies an indexed Cap'n Proto list into a plain slice.
*/
func readList(length int, at func(int) float64) []float64 {
	values := make([]float64, length)

	for index := range values {
		values[index] = at(index)
	}

	return values
}

/*
step drives one turn of the estimation loop: the stepper proposes a point,
the parameter map decodes it, the likelihood is measured there, and the
objective carries that measurement back into the stepper's coordinates.
*/
func step(
	t *testing.T,
	stepper optimization.LBFGS,
	seed []float64,
	value float64,
	gradient []float64,
	tolerance float64,
) ([]float64, bool) {
	t.Helper()
	ctx := context.Background()

	err := stepper.Write(ctx, func(params optimization.LBFGS_write_Params) error {
		if err := writeFloats(params.NewGradient, gradient); err != nil {
			return err
		}

		if err := writeFloats(params.NewSeed, seed); err != nil {
			return err
		}

		params.SetFVal(value)
		params.SetMemory(8)
		params.SetTolerance(tolerance)
		return nil
	})

	if err != nil {
		t.Fatalf("stepper write: %v", err)
	}

	if err := stepper.WaitStreaming(); err != nil {
		t.Fatalf("stepper stream: %v", err)
	}

	future, release := stepper.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("stepper done: %v", err)
	}

	list, err := results.X()

	if err != nil {
		t.Fatalf("stepper x: %v", err)
	}

	return readList(list.Len(), list.At), results.Converged()
}

/*
present drives the Objective node, turning a likelihood and its gradient in
natural parameters into something a minimizer can step along.
*/
func present(t *testing.T, value float64, gradient, jacobian []float64) (float64, []float64) {
	t.Helper()
	ctx := context.Background()
	client := optimization.Objective_ServerToClient(optimization.NewObjective())

	err := client.Write(ctx, func(params optimization.Objective_write_Params) error {
		if err := writeFloats(params.NewGradient, gradient); err != nil {
			return err
		}

		if err := writeFloats(params.NewJacobian, jacobian); err != nil {
			return err
		}

		params.SetValue(value)
		params.SetSense(-1)
		return nil
	})

	if err != nil {
		t.Fatalf("objective write: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("objective stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("objective done: %v", err)
	}

	list, err := results.Gradient()

	if err != nil {
		t.Fatalf("objective gradient: %v", err)
	}

	return results.FVal(), readList(list.Len(), list.At)
}

/*
fit runs the estimation loop to convergence and reports the parameters it
settled on. This is the composition the manifest wires: Events into Domain,
Domain and the stepper into Parameters, Parameters into KernelIntegral and
LogLikelihood, and the objective back into the stepper.
*/
func fit(t *testing.T, path realisation, budget int, coupled float64) mapped {
	t.Helper()
	derived := domainFor(t, path, coupled)

	if !derived.defined {
		t.Fatalf("fit: no search region could be derived from the window")
	}

	ctx := context.Background()
	stepper := optimization.LBFGS_ServerToClient(optimization.NewLBFGS(ctx))
	width := len(derived.seed)
	value := 0.0
	gradient := make([]float64, width)
	settled := mapped{}

	for turn := 0; turn < budget; turn++ {
		coordinates, converged := step(t, stepper, derived.seed, value, gradient, 1e-8)

		if converged {
			break
		}

		result := mapCoordinates(t, coordinates, derived.lower, derived.upper, path.dimension)
		settled = result
		measured, natural, defined := likelihood(t, path, result.baseline, result.excitation, result.decay)

		if !defined {
			// A parameter set with no likelihood is refused by making it
			// infinitely expensive, which the line search answers by
			// shortening its step rather than stalling.
			value = math.Inf(1)
			gradient = make([]float64, width)
			continue
		}

		value, gradient = present(t, measured, natural, result.jacobian)
	}

	return settled
}
