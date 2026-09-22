package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
EmaServer calculates the Exponential Moving Average (EMA).
*/
type EmaServer struct {
	*runtime.System
	calculator *trend.Ema[float64]
	value chan float64
	out <-chan float64
	result float64
	count int
}

func NewEma(ctx context.Context) *EmaServer {
	value := make(chan float64, 1)
	calculator := trend.NewEma[float64]()

	server := &EmaServer{
		System: runtime.NewSystem(ctx, "financial.trend.ema"),
		calculator: calculator,
		value: value,
		out: calculator.ComputeWithContext(ctx, value),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *EmaServer) Write(ctx context.Context, call Ema_write) error {
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
					"[financial.trend.ema.Write] calculator channel closed",
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
func (server *EmaServer) Done(ctx context.Context, call Ema_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.ema.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
