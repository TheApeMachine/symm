package integration

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/integrate/quad"
	"math"
)

/*
GaussHermiteServer calculates fixed Gauss-Hermite quadrature integral over infinite support (-inf, +inf).
*/
type GaussHermiteServer struct {
	*runtime.System
	integral float64
}

func NewGaussHermite(ctx context.Context) *GaussHermiteServer {
	server := &GaussHermiteServer{
		System: runtime.NewSystem(ctx, "integration.gauss_hermite"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and calculation parameters.
*/
func (server *GaussHermiteServer) Write(ctx context.Context, call GaussHermite_write) error {
	args := call.Args()
	points := int(args.Points())
	coeffList, err := args.Coeffs()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read coeffs", err))
	}

	if points <= 0 {
		points = 10
	}

	coeffs := make([]float64, coeffList.Len())
	for index := 0; index < coeffList.Len(); index++ {
		coeffs[index] = coeffList.At(index)
	}

	polyFn := func(val float64) float64 {
		res := 0.0
		pow := 1.0
		for _, coeff := range coeffs {
			res += coeff * pow
			pow *= val
		}
		return res
	}

	server.integral = quad.Fixed(polyFn, math.Inf(-1), math.Inf(1), points, quad.Hermite{}, 0)
	return nil
}

/*
Done returns calculated integration results.
*/
func (server *GaussHermiteServer) Done(ctx context.Context, call GaussHermite_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[integration.gauss_hermite.Done] failed to allocate results", err))
	}
	results.SetIntegral(server.integral)
	return nil
}
