package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
RmaServer calculates the Running Moving Average (RMA).
*/
type RmaServer struct {
	*runtime.System
	calculator *trend.Rma[float64]
	value      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewRma(ctx context.Context) *RmaServer {
	value := make(chan float64, 1)
	calculator := trend.NewRma[float64]()

	server := &RmaServer{
		System:     runtime.NewSystem(ctx, "financial.trend.rma"),
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
func (server *RmaServer) Write(ctx context.Context, call Rma_write) error {
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
					"[financial.trend.rma.Write] calculator channel closed",
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
func (server *RmaServer) Done(ctx context.Context, call Rma_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.rma.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
