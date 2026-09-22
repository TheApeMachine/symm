package hawkes

import (
	"context"
	"math"
	"math/cmplx"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"gonum.org/v1/gonum/mat"
)

/*
SpectralRadiusServer returns the largest eigenvalue modulus of a square
matrix. Applied to a branching matrix this is the criticality of the
process: strictly below one every cascade of excitation dies out and the
process is stationary; at or above one the expected progeny of a single
arrival diverges.
*/
type SpectralRadiusServer struct {
	radius float64
}

func NewSpectralRadius() *SpectralRadiusServer {
	return &SpectralRadiusServer{}
}

func (server *SpectralRadiusServer) Write(ctx context.Context, call SpectralRadius_write) error {
	args := call.Args()
	values, err := server.read(args.Matrix())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes spectral radius: failed to read matrix",
			err,
		))
	}

	dimension := int(args.Dimension())

	if dimension <= 0 || len(values) < dimension*dimension {
		server.radius = 0
		return nil
	}

	var decomposition mat.Eigen

	if !decomposition.Factorize(mat.NewDense(dimension, dimension, values[:dimension*dimension]), mat.EigenNone) {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"hawkes spectral radius: eigendecomposition did not converge",
			nil,
		))
	}

	radius := 0.0

	for _, eigenvalue := range decomposition.Values(nil) {
		radius = math.Max(radius, cmplx.Abs(eigenvalue))
	}

	server.radius = radius
	return nil
}

/*
read copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *SpectralRadiusServer) read(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

func (server *SpectralRadiusServer) Done(ctx context.Context, call SpectralRadius_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes spectral radius: failed to allocate results",
			err,
		))
	}

	results.SetRadius(server.radius)
	return nil
}
