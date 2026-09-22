package volatility

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volatility"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
AtrServer calculates the Average True Range (ATR).
*/
type AtrServer struct {
	*runtime.System
	calculator *indicator.Atr[float64]
	high chan float64
	low chan float64
	close chan float64
	out <-chan float64
	result float64
	count int
}

func NewAtr(ctx context.Context) *AtrServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := indicator.NewAtr[float64]()

	server := &AtrServer{
		System: runtime.NewSystem(ctx, "financial.volatility.atr"),
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
func (server *AtrServer) Write(ctx context.Context, call Atr_write) error {
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
					"[financial.volatility.atr.Write] calculator channel closed",
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
func (server *AtrServer) Done(ctx context.Context, call Atr_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volatility.atr.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
