package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
KdjServer calculates the KDJ indicator (%K, %D, %J).
*/
type KdjServer struct {
	*runtime.System
	calculator *trend.Kdj[float64]
	high chan float64
	low chan float64
	close chan float64
	kOut <-chan float64
	dOut <-chan float64
	jOut <-chan float64
	k float64
	d float64
	j float64
	count int
}

func NewKdj(ctx context.Context) *KdjServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := trend.NewKdj[float64]()

	server := &KdjServer{
		System: runtime.NewSystem(ctx, "financial.trend.kdj"),
		calculator: calculator,
		high: high,
		low: low,
		close: close,
	}

	server.kOut, server.dOut, server.jOut = calculator.ComputeWithContext(ctx, high, low, close)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *KdjServer) Write(ctx context.Context, call Kdj_write) error {
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
			case res, ok := <-server.kOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.kdj.Write] k channel closed",
						nil,
					))
				}

				server.k = res
				readFirst = true
			case res, ok := <-server.dOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.kdj.Write] d channel closed",
						nil,
					))
				}

				server.d = res
				readSecond = true
			case res, ok := <-server.jOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.kdj.Write] j channel closed",
						nil,
					))
				}

				server.j = res
				readThird = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *KdjServer) Done(ctx context.Context, call Kdj_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.kdj.Done] failed to allocate done results",
			err,
		))
	}

	results.SetK(server.k)
	results.SetD(server.d)
	results.SetJ(server.j)
	return nil
}
