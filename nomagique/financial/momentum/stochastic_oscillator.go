package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
StochasticOscillatorServer calculates the Stochastic Oscillator.
*/
type StochasticOscillatorServer struct {
	*runtime.System
	calculator *indicator.StochasticOscillator[float64]
	high chan float64
	low chan float64
	close chan float64
	kOut <-chan float64
	k float64
	dOut <-chan float64
	d float64
	count int
}

func NewStochasticOscillator(ctx context.Context) *StochasticOscillatorServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := indicator.NewStochasticOscillator[float64]()

	server := &StochasticOscillatorServer{
		System: runtime.NewSystem(ctx, "financial.momentum.stochastic_oscillator"),
		calculator: calculator,
		high: high,
		low: low,
		close: close,
	}

	server.kOut, server.dOut = calculator.ComputeWithContext(ctx, high, low, close)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *StochasticOscillatorServer) Write(ctx context.Context, call StochasticOscillator_write) error {
	highVal := call.Args().High()
	lowVal := call.Args().Low()
	closeVal := call.Args().Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.high <- highVal:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.low <- lowVal:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.close <- closeVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		readK := false
		readD := false

		for !readK || !readD {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.kOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.stochastic_oscillator.Write] k channel closed",
						nil,
					))
				}

				server.k = res
				readK = true
			case res, ok := <-server.dOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.stochastic_oscillator.Write] d channel closed",
						nil,
					))
				}

				server.d = res
				readD = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *StochasticOscillatorServer) Done(ctx context.Context, call StochasticOscillator_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.stochastic_oscillator.Done] failed to allocate done results",
			err,
		))
	}

	results.SetK(server.k)
	results.SetD(server.d)
	return nil
}
