package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
SmmaServer calculates the Smoothed Moving Average (SMMA).
*/
type SmmaServer struct {
	*runtime.System
	calculator *trend.Smma[float64]
	value      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewSmma(ctx context.Context) *SmmaServer {
	value := make(chan float64, 1)
	calculator := trend.NewSmma[float64]()

	server := &SmmaServer{
		System:     runtime.NewSystem(ctx, "financial.trend.smma"),
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
func (server *SmmaServer) Write(ctx context.Context, call Smma_write) error {
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
					"[financial.trend.smma.Write] calculator channel closed",
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
func (server *SmmaServer) Done(ctx context.Context, call Smma_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.smma.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
