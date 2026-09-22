package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
TrixServer calculates the TRIX indicator.
*/
type TrixServer struct {
	*runtime.System
	calculator *trend.Trix[float64]
	value chan float64
	out <-chan float64
	result float64
	count int
}

func NewTrix(ctx context.Context) *TrixServer {
	value := make(chan float64, 1)
	calculator := trend.NewTrix[float64]()

	server := &TrixServer{
		System: runtime.NewSystem(ctx, "financial.trend.trix"),
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
func (server *TrixServer) Write(ctx context.Context, call Trix_write) error {
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
					"[financial.trend.trix.Write] calculator channel closed",
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
func (server *TrixServer) Done(ctx context.Context, call Trix_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.trix.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
