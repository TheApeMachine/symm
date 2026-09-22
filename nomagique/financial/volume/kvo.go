package volume

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volume"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
KvoServer calculates the Klinger Volume Oscillator (KVO) and signal line.
*/
type KvoServer struct {
	*runtime.System
	calculator *indicator.Kvo[float64]
	high       chan float64
	low        chan float64
	volume     chan float64
	kvoOut     <-chan float64
	signalOut  <-chan float64
	kvo        float64
	signal     float64
	count      int
}

func NewKvo(ctx context.Context) *KvoServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	volume := make(chan float64, 1)
	calculator := indicator.NewKvo[float64]()

	server := &KvoServer{
		System:     runtime.NewSystem(ctx, "financial.volume.kvo"),
		calculator: calculator,
		high:       high,
		low:        low,
		volume:     volume,
	}

	server.kvoOut, server.signalOut = calculator.ComputeWithContext(ctx, high, low, volume)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *KvoServer) Write(ctx context.Context, call Kvo_write) error {
	highVal := call.Args().High()
	lowVal := call.Args().Low()
	volumeVal := call.Args().Volume()

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
	case server.volume <- volumeVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		readFirst := false
		readSecond := false

		for !readFirst || !readSecond {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.kvoOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.volume.kvo.Write] kvo channel closed",
						nil,
					))
				}

				server.kvo = res
				readFirst = true
			case res, ok := <-server.signalOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.volume.kvo.Write] signal channel closed",
						nil,
					))
				}

				server.signal = res
				readSecond = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *KvoServer) Done(ctx context.Context, call Kvo_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volume.kvo.Done] failed to allocate done results",
			err,
		))
	}

	results.SetKvo(server.kvo)
	results.SetSignal(server.signal)
	return nil
}
