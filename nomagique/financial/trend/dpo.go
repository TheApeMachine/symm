package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
DpoServer calculates the Detrended Price Oscillator (DPO).
*/
type DpoServer struct {
	*runtime.System
	calculator *trend.Dpo[float64]
	value      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewDpo(ctx context.Context) *DpoServer {
	value := make(chan float64, 1)
	calculator := trend.NewDpo[float64]()

	server := &DpoServer{
		System:     runtime.NewSystem(ctx, "financial.trend.dpo"),
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
func (server *DpoServer) Write(ctx context.Context, call Dpo_write) error {
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
					"[financial.trend.dpo.Write] calculator channel closed",
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
func (server *DpoServer) Done(ctx context.Context, call Dpo_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.dpo.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
