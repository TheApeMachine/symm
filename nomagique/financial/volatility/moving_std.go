package volatility

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volatility"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MovingStdServer calculates the Moving Standard Deviation.
*/
type MovingStdServer struct {
	*runtime.System
	calculator *indicator.MovingStd[float64]
	value      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewMovingStd(ctx context.Context) *MovingStdServer {
	value := make(chan float64, 1)
	calculator := indicator.NewMovingStd[float64]()

	server := &MovingStdServer{
		System:     runtime.NewSystem(ctx, "financial.volatility.moving_std"),
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
func (server *MovingStdServer) Write(ctx context.Context, call MovingStd_write) error {
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
					"[financial.volatility.moving_std.Write] calculator channel closed",
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
func (server *MovingStdServer) Done(ctx context.Context, call MovingStd_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volatility.moving_std.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
