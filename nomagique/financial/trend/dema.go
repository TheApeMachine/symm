package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
DemaServer calculates the Double Exponential Moving Average (DEMA).
*/
type DemaServer struct {
	*runtime.System
	calculator *trend.Dema[float64]
	value chan float64
	out <-chan float64
	result float64
	count int
}

func NewDema(ctx context.Context) *DemaServer {
	value := make(chan float64, 1)
	calculator := trend.NewDema[float64]()

	server := &DemaServer{
		System: runtime.NewSystem(ctx, "financial.trend.dema"),
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
func (server *DemaServer) Write(ctx context.Context, call Dema_write) error {
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
					"[financial.trend.dema.Write] calculator channel closed",
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
func (server *DemaServer) Done(ctx context.Context, call Dema_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.dema.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
