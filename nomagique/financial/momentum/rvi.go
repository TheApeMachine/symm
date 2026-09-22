package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
RviServer calculates the Relative Volatility Index (RVI).
*/
type RviServer struct {
	*runtime.System
	calculator *indicator.Rvi[float64]
	open chan float64
	high chan float64
	low chan float64
	close chan float64
	rviOut <-chan float64
	rvi float64
	signalOut <-chan float64
	signal float64
	count int
}

func NewRvi(ctx context.Context) *RviServer {
	open := make(chan float64, 1)
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := indicator.NewRvi[float64]()

	server := &RviServer{
		System: runtime.NewSystem(ctx, "financial.momentum.rvi"),
		calculator: calculator,
		open: open,
		high: high,
		low: low,
		close: close,
	}

	server.rviOut, server.signalOut = calculator.ComputeWithContext(ctx, open, high, low, close)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *RviServer) Write(ctx context.Context, call Rvi_write) error {
	openVal := call.Args().Open()
	highVal := call.Args().High()
	lowVal := call.Args().Low()
	closeVal := call.Args().Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.open <- openVal:
	}

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
		readRvi := false
		readSignal := false

		for !readRvi || !readSignal {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.rviOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.rvi.Write] rvi channel closed",
						nil,
					))
				}

				server.rvi = res
				readRvi = true
			case res, ok := <-server.signalOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.rvi.Write] signal channel closed",
						nil,
					))
				}

				server.signal = res
				readSignal = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *RviServer) Done(ctx context.Context, call Rvi_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.rvi.Done] failed to allocate done results",
			err,
		))
	}

	results.SetRvi(server.rvi)
	results.SetSignal(server.signal)
	return nil
}
