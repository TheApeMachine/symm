package volatility

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volatility"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
AccelerationBandsServer calculates the Acceleration Bands.
*/
type AccelerationBandsServer struct {
	*runtime.System
	calculator *indicator.AccelerationBands[float64]
	high       chan float64
	low        chan float64
	close      chan float64
	upperOut   <-chan float64
	middleOut  <-chan float64
	lowerOut   <-chan float64
	upper      float64
	middle     float64
	lower      float64
	count      int
}

func NewAccelerationBands(ctx context.Context) *AccelerationBandsServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := indicator.NewAccelerationBands[float64]()

	server := &AccelerationBandsServer{
		System:     runtime.NewSystem(ctx, "financial.volatility.acceleration_bands"),
		calculator: calculator,
		high:       high,
		low:        low,
		close:      close,
	}

	server.upperOut, server.middleOut, server.lowerOut = calculator.ComputeWithContext(ctx, high, low, close)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *AccelerationBandsServer) Write(ctx context.Context, call AccelerationBands_write) error {
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
		readThird := false

		for !readFirst || !readSecond || !readThird {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.upperOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.volatility.acceleration_bands.Write] upper channel closed",
						nil,
					))
				}

				server.upper = res
				readFirst = true
			case res, ok := <-server.middleOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.volatility.acceleration_bands.Write] middle channel closed",
						nil,
					))
				}

				server.middle = res
				readSecond = true
			case res, ok := <-server.lowerOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.volatility.acceleration_bands.Write] lower channel closed",
						nil,
					))
				}

				server.lower = res
				readThird = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *AccelerationBandsServer) Done(ctx context.Context, call AccelerationBands_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volatility.acceleration_bands.Done] failed to allocate done results",
			err,
		))
	}

	results.SetUpper(server.upper)
	results.SetMiddle(server.middle)
	results.SetLower(server.lower)
	return nil
}
