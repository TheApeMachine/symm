package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MacdServer calculates Moving Average Convergence Divergence (MACD) and signal.
*/
type MacdServer struct {
	*runtime.System
	calculator *trend.Macd[float64]
	value      chan float64
	macdOut    <-chan float64
	signalOut  <-chan float64
	macd       float64
	signal     float64
	count      int
}

func NewMacd(ctx context.Context) *MacdServer {
	value := make(chan float64, 1)
	calculator := trend.NewMacd[float64]()

	server := &MacdServer{
		System:     runtime.NewSystem(ctx, "financial.trend.macd"),
		calculator: calculator,
		value:      value,
	}

	server.macdOut, server.signalOut = calculator.ComputeWithContext(ctx, value)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *MacdServer) Write(ctx context.Context, call Macd_write) error {
	valueVal := call.Args().Value()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.value <- valueVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		readFirst := false
		readSecond := false

		for !readFirst || !readSecond {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.macdOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.macd.Write] macd channel closed",
						nil,
					))
				}

				server.macd = res
				readFirst = true
			case res, ok := <-server.signalOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.macd.Write] signal channel closed",
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
func (server *MacdServer) Done(ctx context.Context, call Macd_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.macd.Done] failed to allocate done results",
			err,
		))
	}

	results.SetMacd(server.macd)
	results.SetSignal(server.signal)
	return nil
}
