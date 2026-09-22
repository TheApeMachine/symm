package volatility

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volatility"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
KeltnerChannelServer calculates the Keltner Channel.
*/
type KeltnerChannelServer struct {
	*runtime.System
	calculator *indicator.KeltnerChannel[float64]
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

func NewKeltnerChannel(ctx context.Context) *KeltnerChannelServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := indicator.NewKeltnerChannel[float64]()

	server := &KeltnerChannelServer{
		System:     runtime.NewSystem(ctx, "financial.volatility.keltner_channel"),
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
func (server *KeltnerChannelServer) Write(ctx context.Context, call KeltnerChannel_write) error {
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
						"[financial.volatility.keltner_channel.Write] upper channel closed",
						nil,
					))
				}

				server.upper = res
				readFirst = true
			case res, ok := <-server.middleOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.volatility.keltner_channel.Write] middle channel closed",
						nil,
					))
				}

				server.middle = res
				readSecond = true
			case res, ok := <-server.lowerOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.volatility.keltner_channel.Write] lower channel closed",
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
func (server *KeltnerChannelServer) Done(ctx context.Context, call KeltnerChannel_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volatility.keltner_channel.Done] failed to allocate done results",
			err,
		))
	}

	results.SetUpper(server.upper)
	results.SetMiddle(server.middle)
	results.SetLower(server.lower)
	return nil
}
