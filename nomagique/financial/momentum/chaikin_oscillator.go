package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ChaikinOscillatorServer calculates the Chaikin Oscillator.
*/
type ChaikinOscillatorServer struct {
	*runtime.System
	calculator *indicator.ChaikinOscillator[float64]
	high       chan float64
	low        chan float64
	close      chan float64
	volume     chan float64
	coOut      <-chan float64
	co         float64
	adOut      <-chan float64
	ad         float64
	count      int
}

func NewChaikinOscillator(ctx context.Context) *ChaikinOscillatorServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	volume := make(chan float64, 1)
	calculator := indicator.NewChaikinOscillator[float64]()

	server := &ChaikinOscillatorServer{
		System:     runtime.NewSystem(ctx, "financial.momentum.chaikin_oscillator"),
		calculator: calculator,
		high:       high,
		low:        low,
		close:      close,
		volume:     volume,
	}

	server.coOut, server.adOut = calculator.ComputeWithContext(ctx, high, low, close, volume)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *ChaikinOscillatorServer) Write(ctx context.Context, call ChaikinOscillator_write) error {
	highVal := call.Args().High()
	lowVal := call.Args().Low()
	closeVal := call.Args().Close()
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
	case server.close <- closeVal:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.volume <- volumeVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		readCo := false
		readAd := false

		for !readCo || !readAd {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.coOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.chaikin_oscillator.Write] co channel closed",
						nil,
					))
				}

				server.co = res
				readCo = true
			case res, ok := <-server.adOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.chaikin_oscillator.Write] ad channel closed",
						nil,
					))
				}

				server.ad = res
				readAd = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *ChaikinOscillatorServer) Done(ctx context.Context, call ChaikinOscillator_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.chaikin_oscillator.Done] failed to allocate done results",
			err,
		))
	}

	results.SetCo(server.co)
	results.SetAd(server.ad)
	return nil
}
