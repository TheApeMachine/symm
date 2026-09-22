package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
WilliamsRServer calculates the Williams %R.
*/
type WilliamsRServer struct {
	*runtime.System
	calculator *indicator.WilliamsR[float64]
	high chan float64
	low chan float64
	close chan float64
	out <-chan float64
	result float64
	count int
}

func NewWilliamsR(ctx context.Context) *WilliamsRServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := indicator.NewWilliamsR[float64]()

	server := &WilliamsRServer{
		System: runtime.NewSystem(ctx, "financial.momentum.williams_r"),
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
func (server *WilliamsRServer) Write(ctx context.Context, call WilliamsR_write) error {
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
					"[financial.momentum.williams_r.Write] calculator channel closed",
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
func (server *WilliamsRServer) Done(ctx context.Context, call WilliamsR_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.williams_r.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
