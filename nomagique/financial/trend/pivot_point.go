package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
PivotPointServer calculates standard Pivot Point support and resistance levels.
*/
type PivotPointServer struct {
	*runtime.System
	calculator *trend.PivotPoint[float64]
	open chan float64
	high chan float64
	low chan float64
	close chan float64
	out <-chan trend.PivotPointResult[float64]
	result trend.PivotPointResult[float64]
	count int
}

func NewPivotPoint(ctx context.Context) *PivotPointServer {
	open := make(chan float64, 1)
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := trend.NewPivotPoint[float64]()

	server := &PivotPointServer{
		System: runtime.NewSystem(ctx, "financial.trend.pivot_point"),
		calculator: calculator,
		open: open,
		high: high,
		low: low,
		close: close,
		out: calculator.ComputeWithContext(ctx, open, high, low, close),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *PivotPointServer) Write(ctx context.Context, call PivotPoint_write) error {
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
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res, ok := <-server.out:
			if !ok {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"[financial.trend.pivot_point.Write] calculator channel closed",
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
func (server *PivotPointServer) Done(ctx context.Context, call PivotPoint_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.pivot_point.Done] failed to allocate done results",
			err,
		))
	}

	results.SetP(server.result.P)
	results.SetR1(server.result.R1)
	results.SetR2(server.result.R2)
	results.SetR3(server.result.R3)
	results.SetR4(server.result.R4)
	results.SetS1(server.result.S1)
	results.SetS2(server.result.S2)
	results.SetS3(server.result.S3)
	results.SetS4(server.result.S4)
	return nil
}
