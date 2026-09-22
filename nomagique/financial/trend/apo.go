package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ApoServer calculates the Absolute Price Oscillator (APO).
*/
type ApoServer struct {
	*runtime.System
	calculator *trend.Apo[float64]
	value      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewApo(ctx context.Context) *ApoServer {
	value := make(chan float64, 1)
	calculator := trend.NewApo[float64]()

	server := &ApoServer{
		System:     runtime.NewSystem(ctx, "financial.trend.apo"),
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
func (server *ApoServer) Write(ctx context.Context, call Apo_write) error {
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
					"[financial.trend.apo.Write] calculator channel closed",
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
func (server *ApoServer) Done(ctx context.Context, call Apo_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.apo.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
