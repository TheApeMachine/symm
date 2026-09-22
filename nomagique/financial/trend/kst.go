package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
KstServer calculates the Know Sure Thing (KST) oscillator and signal.
*/
type KstServer struct {
	*runtime.System
	calculator *trend.Kst[float64]
	value chan float64
	kstOut <-chan float64
	signalOut <-chan float64
	kst float64
	signal float64
	count int
}

func NewKst(ctx context.Context) *KstServer {
	value := make(chan float64, 1)
	calculator := trend.NewKst[float64]()

	server := &KstServer{
		System: runtime.NewSystem(ctx, "financial.trend.kst"),
		calculator: calculator,
		value: value,
	}

	server.kstOut, server.signalOut = calculator.ComputeWithContext(ctx, value)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *KstServer) Write(ctx context.Context, call Kst_write) error {
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
			case res, ok := <-server.kstOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.kst.Write] kst channel closed",
						nil,
					))
				}

				server.kst = res
				readFirst = true
			case res, ok := <-server.signalOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.kst.Write] signal channel closed",
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
func (server *KstServer) Done(ctx context.Context, call Kst_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.kst.Done] failed to allocate done results",
			err,
		))
	}

	results.SetKst(server.kst)
	results.SetSignal(server.signal)
	return nil
}
