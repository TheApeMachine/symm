package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
SmaServer calculates the Simple Moving Average (SMA).
*/
type SmaServer struct {
	*runtime.System
	calculator *trend.Sma[float64]
	value      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewSma(ctx context.Context) *SmaServer {
	value := make(chan float64, 1)
	calculator := trend.NewSma[float64]()

	server := &SmaServer{
		System:     runtime.NewSystem(ctx, "financial.trend.sma"),
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
func (server *SmaServer) Write(ctx context.Context, call Sma_write) error {
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
					"[financial.trend.sma.Write] calculator channel closed",
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
func (server *SmaServer) Done(ctx context.Context, call Sma_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.sma.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
