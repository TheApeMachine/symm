package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
HmaServer calculates the Hull Moving Average (HMA).
*/
type HmaServer struct {
	*runtime.System
	calculator *trend.Hma[float64]
	value      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewHma(ctx context.Context) *HmaServer {
	value := make(chan float64, 1)
	calculator := trend.NewHma[float64]()

	server := &HmaServer{
		System:     runtime.NewSystem(ctx, "financial.trend.hma"),
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
func (server *HmaServer) Write(ctx context.Context, call Hma_write) error {
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
					"[financial.trend.hma.Write] calculator channel closed",
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
func (server *HmaServer) Done(ctx context.Context, call Hma_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.hma.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
