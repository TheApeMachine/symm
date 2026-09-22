package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MovingMaxServer calculates the Moving Maximum.
*/
type MovingMaxServer struct {
	*runtime.System
	calculator *trend.MovingMax[float64]
	value chan float64
	out <-chan float64
	result float64
	count int
}

func NewMovingMax(ctx context.Context) *MovingMaxServer {
	value := make(chan float64, 1)
	calculator := trend.NewMovingMax[float64]()

	server := &MovingMaxServer{
		System: runtime.NewSystem(ctx, "financial.trend.moving_max"),
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
func (server *MovingMaxServer) Write(ctx context.Context, call MovingMax_write) error {
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
					"[financial.trend.moving_max.Write] calculator channel closed",
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
func (server *MovingMaxServer) Done(ctx context.Context, call MovingMax_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.moving_max.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
