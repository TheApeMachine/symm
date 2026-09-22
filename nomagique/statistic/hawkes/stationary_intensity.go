package hawkes

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"gonum.org/v1/gonum/mat"
)

/*
StationaryIntensityServer solves (identity - branching) * intensity =
baseline. The baseline is the rate at which each component produces arrivals
on its own; the solution is the rate it settles at once those arrivals'
descendants are counted too. It exists only for a subcritical process, where
that series converges.
*/
type StationaryIntensityServer struct {
	intensity []float64
	defined   bool
}

func NewStationaryIntensity() *StationaryIntensityServer {
	return &StationaryIntensityServer{}
}

func (server *StationaryIntensityServer) Write(ctx context.Context, call StationaryIntensity_write) error {
	args := call.Args()
	baseline, err := server.read(args.Baseline())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes stationary intensity: failed to read baseline",
			err,
		))
	}

	branching, err := server.read(args.Branching())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes stationary intensity: failed to read branching",
			err,
		))
	}

	dimension := int(args.Dimension())
	server.intensity = nil
	server.defined = false

	if dimension <= 0 || len(baseline) < dimension || len(branching) < dimension*dimension {
		return nil
	}

	residual := mat.NewDense(dimension, dimension, nil)

	for row := 0; row < dimension; row++ {
		for column := 0; column < dimension; column++ {
			identity := 0.0

			if row == column {
				identity = 1
			}

			residual.Set(row, column, identity-branching[row*dimension+column])
		}
	}

	var solution mat.VecDense

	if err := solution.SolveVec(residual, mat.NewVecDense(dimension, baseline[:dimension])); err != nil {
		return nil
	}

	intensity := make([]float64, dimension)

	for index := range intensity {
		intensity[index] = solution.AtVec(index)

		if intensity[index] < 0 || math.IsNaN(intensity[index]) || math.IsInf(intensity[index], 0) {
			return nil
		}
	}

	server.intensity = intensity
	server.defined = true
	return nil
}

/*
read copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *StationaryIntensityServer) read(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

func (server *StationaryIntensityServer) Done(ctx context.Context, call StationaryIntensity_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes stationary intensity: failed to allocate results",
			err,
		))
	}

	list, err := results.NewIntensity(int32(len(server.intensity)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes stationary intensity: failed to allocate intensity list",
			err,
		))
	}

	for index, value := range server.intensity {
		list.Set(index, value)
	}

	results.SetDefined(server.defined)
	return nil
}
