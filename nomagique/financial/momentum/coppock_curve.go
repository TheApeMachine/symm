package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
CoppockCurveServer calculates the Coppock Curve.
*/
type CoppockCurveServer struct {
	*runtime.System
	calculator *indicator.CoppockCurve[float64]
	value      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewCoppockCurve(ctx context.Context) *CoppockCurveServer {
	value := make(chan float64, 1)
	calculator := indicator.NewCoppockCurve[float64]()

	server := &CoppockCurveServer{
		System:     runtime.NewSystem(ctx, "financial.momentum.coppock_curve"),
		calculator: calculator,
		value:      value,
		out:        calculator.ComputeWithContext(ctx, value),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *CoppockCurveServer) Write(ctx context.Context, call CoppockCurve_write) error {
	valueVal := call.Args().Value()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.value <- valueVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res, ok := <-server.out:
			if !ok {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"[financial.momentum.coppock_curve.Write] calculator channel closed",
					nil,
				))
			}

			server.result = res
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *CoppockCurveServer) Done(ctx context.Context, call CoppockCurve_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.coppock_curve.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
