package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
UltimateOscillatorServer calculates the Ultimate Oscillator (UO).
*/
type UltimateOscillatorServer struct {
	*runtime.System
	calculator *indicator.UltimateOscillator[float64]
	high chan float64
	low chan float64
	close chan float64
	out <-chan float64
	result float64
	count int
}

func NewUltimateOscillator(ctx context.Context) *UltimateOscillatorServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := indicator.NewUltimateOscillator[float64]()

	server := &UltimateOscillatorServer{
		System: runtime.NewSystem(ctx, "financial.momentum.ultimate_oscillator"),
		calculator: calculator,
		high: high,
		low: low,
		close: close,
		out: calculator.ComputeWithContext(ctx, high, low, close),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *UltimateOscillatorServer) Write(ctx context.Context, call UltimateOscillator_write) error {
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
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res, ok := <-server.out:
			if !ok {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"[financial.momentum.ultimate_oscillator.Write] calculator channel closed",
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
func (server *UltimateOscillatorServer) Done(ctx context.Context, call UltimateOscillator_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.ultimate_oscillator.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
