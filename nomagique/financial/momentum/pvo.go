package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
PvoServer calculates the Percentage Volume Oscillator (PVO).
*/
type PvoServer struct {
	*runtime.System
	calculator *indicator.Pvo[float64]
	volume chan float64
	pvoOut <-chan float64
	pvo float64
	signalOut <-chan float64
	signal float64
	histogramOut <-chan float64
	histogram float64
	count int
}

func NewPvo(ctx context.Context) *PvoServer {
	volume := make(chan float64, 1)
	calculator := indicator.NewPvo[float64]()

	server := &PvoServer{
		System: runtime.NewSystem(ctx, "financial.momentum.pvo"),
		calculator: calculator,
		volume: volume,
	}

	server.pvoOut, server.signalOut, server.histogramOut = calculator.ComputeWithContext(ctx, volume)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *PvoServer) Write(ctx context.Context, call Pvo_write) error {
	volumeVal := call.Args().Volume()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.volume <- volumeVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		readPvo := false
		readSignal := false
		readHistogram := false

		for !readPvo || !readSignal || !readHistogram {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.pvoOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.pvo.Write] pvo channel closed",
						nil,
					))
				}

				server.pvo = res
				readPvo = true
			case res, ok := <-server.signalOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.pvo.Write] signal channel closed",
						nil,
					))
				}

				server.signal = res
				readSignal = true
			case res, ok := <-server.histogramOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.pvo.Write] histogram channel closed",
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
func (server *PvoServer) Done(ctx context.Context, call Pvo_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.pvo.Done] failed to allocate done results",
			err,
		))
	}

	results.SetPvo(server.pvo)
	results.SetSignal(server.signal)
	results.SetHistogram(server.histogram)
	return nil
}
