package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
SlowStochasticServer calculates the Slow Stochastic %K and %D.
*/
type SlowStochasticServer struct {
	*runtime.System
	calculator *trend.SlowStochastic[float64]
	value chan float64
	kOut <-chan float64
	dOut <-chan float64
	k float64
	d float64
	count int
}

func NewSlowStochastic(ctx context.Context) *SlowStochasticServer {
	value := make(chan float64, 1)
	calculator := trend.NewSlowStochastic[float64]()

	server := &SlowStochasticServer{
		System: runtime.NewSystem(ctx, "financial.trend.slow_stochastic"),
		calculator: calculator,
		value: value,
	}

	server.kOut, server.dOut = calculator.ComputeWithContext(ctx, value)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *SlowStochasticServer) Write(ctx context.Context, call SlowStochastic_write) error {
	valueVal := call.Args().Value()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.value <- valueVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		readFirst := false
		readSecond := false

		for !readFirst || !readSecond {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.kOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.slow_stochastic.Write] k channel closed",
						nil,
					))
				}

				server.k = res
				readFirst = true
			case res, ok := <-server.dOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.slow_stochastic.Write] d channel closed",
						nil,
					))
				}

				server.d = res
				readSecond = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *SlowStochasticServer) Done(ctx context.Context, call SlowStochastic_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.slow_stochastic.Done] failed to allocate done results",
			err,
		))
	}

	results.SetK(server.k)
	results.SetD(server.d)
	return nil
}
