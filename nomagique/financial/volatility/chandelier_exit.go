package volatility

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volatility"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ChandelierExitServer calculates the Chandelier Exit.
*/
type ChandelierExitServer struct {
	*runtime.System
	calculator   *indicator.ChandelierExit[float64]
	high         chan float64
	low          chan float64
	close        chan float64
	exitLongOut  <-chan float64
	exitShortOut <-chan float64
	exitLong     float64
	exitShort    float64
	count        int
}

func NewChandelierExit(ctx context.Context) *ChandelierExitServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := indicator.NewChandelierExit[float64]()

	server := &ChandelierExitServer{
		System:     runtime.NewSystem(ctx, "financial.volatility.chandelier_exit"),
		calculator: calculator,
		high:       high,
		low:        low,
		close:      close,
	}

	server.exitLongOut, server.exitShortOut = calculator.ComputeWithContext(ctx, high, low, close)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *ChandelierExitServer) Write(ctx context.Context, call ChandelierExit_write) error {
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
		readFirst := false
		readSecond := false

		for !readFirst || !readSecond {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.exitLongOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.volatility.chandelier_exit.Write] exitLong channel closed",
						nil,
					))
				}

				server.exitLong = res
				readFirst = true
			case res, ok := <-server.exitShortOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.volatility.chandelier_exit.Write] exitShort channel closed",
						nil,
					))
				}

				server.exitShort = res
				readSecond = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *ChandelierExitServer) Done(ctx context.Context, call ChandelierExit_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volatility.chandelier_exit.Done] failed to allocate done results",
			err,
		))
	}

	results.SetExitLong(server.exitLong)
	results.SetExitShort(server.exitShort)
	return nil
}
