package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
TrimaServer calculates the Triangular Moving Average (TRIMA).
*/
type TrimaServer struct {
	*runtime.System
	calculator *trend.Trima[float64]
	value      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewTrima(ctx context.Context) *TrimaServer {
	value := make(chan float64, 1)
	calculator := trend.NewTrima[float64]()

	server := &TrimaServer{
		System:     runtime.NewSystem(ctx, "financial.trend.trima"),
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
func (server *TrimaServer) Write(ctx context.Context, call Trima_write) error {
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
					"[financial.trend.trima.Write] calculator channel closed",
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
func (server *TrimaServer) Done(ctx context.Context, call Trima_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.trima.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
