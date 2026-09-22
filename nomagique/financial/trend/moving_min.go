package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MovingMinServer calculates the Moving Minimum.
*/
type MovingMinServer struct {
	*runtime.System
	calculator *trend.MovingMin[float64]
	value chan float64
	out <-chan float64
	result float64
	count int
}

func NewMovingMin(ctx context.Context) *MovingMinServer {
	value := make(chan float64, 1)
	calculator := trend.NewMovingMin[float64]()

	server := &MovingMinServer{
		System: runtime.NewSystem(ctx, "financial.trend.moving_min"),
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
func (server *MovingMinServer) Write(ctx context.Context, call MovingMin_write) error {
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
					"[financial.trend.moving_min.Write] calculator channel closed",
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
func (server *MovingMinServer) Done(ctx context.Context, call MovingMin_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.moving_min.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
