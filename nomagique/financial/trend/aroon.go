package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
AroonServer calculates the Aroon Indicator (Aroon Up and Aroon Down).
*/
type AroonServer struct {
	*runtime.System
	calculator *trend.Aroon[float64]
	high chan float64
	low chan float64
	upOut <-chan float64
	downOut <-chan float64
	up float64
	down float64
	count int
}

func NewAroon(ctx context.Context) *AroonServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	calculator := trend.NewAroon[float64]()

	server := &AroonServer{
		System: runtime.NewSystem(ctx, "financial.trend.aroon"),
		calculator: calculator,
		high: high,
		low: low,
	}

	server.upOut, server.downOut = calculator.ComputeWithContext(ctx, high, low)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *AroonServer) Write(ctx context.Context, call Aroon_write) error {
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
		readFirst := false
		readSecond := false

		for !readFirst || !readSecond {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.upOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.aroon.Write] up channel closed",
						nil,
					))
				}

				server.up = res
				readFirst = true
			case res, ok := <-server.downOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.aroon.Write] down channel closed",
						nil,
					))
				}

				server.down = res
				readSecond = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *AroonServer) Done(ctx context.Context, call Aroon_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.aroon.Done] failed to allocate done results",
			err,
		))
	}

	results.SetUp(server.up)
	results.SetDown(server.down)
	return nil
}
