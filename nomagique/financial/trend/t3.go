package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
T3Server calculates the Tillson T3 Moving Average.
*/
type T3Server struct {
	*runtime.System
	calculator *trend.T3[float64]
	value      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewT3(ctx context.Context) *T3Server {
	value := make(chan float64, 1)
	calculator := trend.NewT3[float64]()

	server := &T3Server{
		System:     runtime.NewSystem(ctx, "financial.trend.t3"),
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
func (server *T3Server) Write(ctx context.Context, call T3_write) error {
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
					"[financial.trend.t3.Write] calculator channel closed",
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
func (server *T3Server) Done(ctx context.Context, call T3_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.t3.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
