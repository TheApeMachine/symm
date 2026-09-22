package integration

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/integrate/quad"
)

/*
GaussLegendreServer calculates fixed Gauss-Legendre quadrature integral.
*/
type GaussLegendreServer struct {
	*runtime.System
	integral float64
}

func NewGaussLegendre(ctx context.Context) *GaussLegendreServer {
	server := &GaussLegendreServer{
		System: runtime.NewSystem(ctx, "integration.gauss_legendre"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and calculation parameters.
*/
func (server *GaussLegendreServer) Write(ctx context.Context, call GaussLegendre_write) error {
	args := call.Args()
	minVal := args.Min()
	maxVal := args.Max()
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

	server.integral = quad.Fixed(polyFn, minVal, maxVal, points, quad.Legendre{}, 0)
	return nil
}

/*
Done returns calculated integration results.
*/
func (server *GaussLegendreServer) Done(ctx context.Context, call GaussLegendre_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[integration.gauss_legendre.Done] failed to allocate results", err))
	}
	results.SetIntegral(server.integral)
	return nil
}
