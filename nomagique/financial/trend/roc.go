package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
RocServer calculates the Rate of Change (ROC).
*/
type RocServer struct {
	*runtime.System
	calculator *trend.Roc[float64]
	value      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewRoc(ctx context.Context) *RocServer {
	value := make(chan float64, 1)
	calculator := trend.NewRoc[float64]()

	server := &RocServer{
		System:     runtime.NewSystem(ctx, "financial.trend.roc"),
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
func (server *RocServer) Write(ctx context.Context, call Roc_write) error {
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
					"[financial.trend.roc.Write] calculator channel closed",
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
func (server *RocServer) Done(ctx context.Context, call Roc_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.roc.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
