package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
TypicalPriceServer calculates the Typical Price.
*/
type TypicalPriceServer struct {
	*runtime.System
	calculator *trend.TypicalPrice[float64]
	high chan float64
	low chan float64
	close chan float64
	out <-chan float64
	result float64
	count int
}

func NewTypicalPrice(ctx context.Context) *TypicalPriceServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := trend.NewTypicalPrice[float64]()

	server := &TypicalPriceServer{
		System: runtime.NewSystem(ctx, "financial.trend.typical_price"),
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
func (server *TypicalPriceServer) Write(ctx context.Context, call TypicalPrice_write) error {
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
					"[financial.trend.typical_price.Write] calculator channel closed",
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
func (server *TypicalPriceServer) Done(ctx context.Context, call TypicalPrice_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.typical_price.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
