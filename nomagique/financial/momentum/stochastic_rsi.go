package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
StochasticRsiServer calculates the Stochastic RSI (StochRSI).
*/
type StochasticRsiServer struct {
	*runtime.System
	calculator *indicator.StochasticRsi[float64]
	close chan float64
	out <-chan float64
	result float64
	count int
}

func NewStochasticRsi(ctx context.Context) *StochasticRsiServer {
	close := make(chan float64, 1)
	calculator := indicator.NewStochasticRsi[float64]()

	server := &StochasticRsiServer{
		System: runtime.NewSystem(ctx, "financial.momentum.stochastic_rsi"),
		calculator: calculator,
		close: close,
		out: calculator.ComputeWithContext(ctx, close),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *StochasticRsiServer) Write(ctx context.Context, call StochasticRsi_write) error {
	closeVal := call.Args().Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.close <- closeVal:
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
					"[financial.momentum.stochastic_rsi.Write] calculator channel closed",
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
func (server *StochasticRsiServer) Done(ctx context.Context, call StochasticRsi_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.stochastic_rsi.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
