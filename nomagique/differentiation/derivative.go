package differentiation

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/diff/fd"
)

/*
DerivativeServer approximates first and second derivatives via finite differences.
*/
type DerivativeServer struct {
	*runtime.System
	deriv  float64
	deriv2 float64
}

func NewDerivative(ctx context.Context) *DerivativeServer {
	server := &DerivativeServer{
		System: runtime.NewSystem(ctx, "differentiation.derivative"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and calculation parameters.
*/
func (server *DerivativeServer) Write(ctx context.Context, call Derivative_write) error {
	args := call.Args()
	valX := args.X()
	coeffList, err := args.Coeffs()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read coeffs", err))
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

	server.deriv = fd.Derivative(polyFn, valX, nil)
	server.deriv2 = fd.Derivative(polyFn, valX, &fd.Settings{Formula: fd.Central2nd})
	return nil
}

/*
Done returns calculated differentiation results.
*/
func (server *DerivativeServer) Done(ctx context.Context, call Derivative_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[differentiation.derivative.Done] failed to allocate results", err))
	}
	results.SetDeriv(server.deriv)
	results.SetDeriv2(server.deriv2)
	return nil
}
