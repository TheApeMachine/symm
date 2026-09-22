package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
AwesomeOscillatorServer calculates the Awesome Oscillator (AO).
*/
type AwesomeOscillatorServer struct {
	*runtime.System
	calculator *indicator.AwesomeOscillator[float64]
	high chan float64
	low chan float64
	out <-chan float64
	result float64
	count int
}

func NewAwesomeOscillator(ctx context.Context) *AwesomeOscillatorServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	calculator := indicator.NewAwesomeOscillator[float64]()

	server := &AwesomeOscillatorServer{
		System: runtime.NewSystem(ctx, "financial.momentum.awesome_oscillator"),
		calculator: calculator,
		high: high,
		low: low,
		out: calculator.ComputeWithContext(ctx, high, low),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *AwesomeOscillatorServer) Write(ctx context.Context, call AwesomeOscillator_write) error {
	highVal := call.Args().High()
	lowVal := call.Args().Low()

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

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res, ok := <-server.out:
			if !ok {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"[financial.momentum.awesome_oscillator.Write] calculator channel closed",
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
func (server *AwesomeOscillatorServer) Done(ctx context.Context, call AwesomeOscillator_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.awesome_oscillator.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
