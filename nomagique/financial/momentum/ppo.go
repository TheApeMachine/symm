package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
PpoServer calculates the Percentage Price Oscillator (PPO).
*/
type PpoServer struct {
	*runtime.System
	calculator *indicator.Ppo[float64]
	close chan float64
	ppoOut <-chan float64
	ppo float64
	signalOut <-chan float64
	signal float64
	histogramOut <-chan float64
	histogram float64
	count int
}

func NewPpo(ctx context.Context) *PpoServer {
	close := make(chan float64, 1)
	calculator := indicator.NewPpo[float64]()

	server := &PpoServer{
		System: runtime.NewSystem(ctx, "financial.momentum.ppo"),
		calculator: calculator,
		close: close,
	}

	server.ppoOut, server.signalOut, server.histogramOut = calculator.ComputeWithContext(ctx, close)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *PpoServer) Write(ctx context.Context, call Ppo_write) error {
	closeVal := call.Args().Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.close <- closeVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		readPpo := false
		readSignal := false
		readHistogram := false

		for !readPpo || !readSignal || !readHistogram {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.ppoOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.ppo.Write] ppo channel closed",
						nil,
					))
				}

				server.ppo = res
				readPpo = true
			case res, ok := <-server.signalOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.ppo.Write] signal channel closed",
						nil,
					))
				}

				server.signal = res
				readSignal = true
			case res, ok := <-server.histogramOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.ppo.Write] histogram channel closed",
						nil,
					))
				}

				server.histogram = res
				readHistogram = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *PpoServer) Done(ctx context.Context, call Ppo_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.ppo.Done] failed to allocate done results",
			err,
		))
	}

	results.SetPpo(server.ppo)
	results.SetSignal(server.signal)
	results.SetHistogram(server.histogram)
	return nil
}
