package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
KamaServer calculates Kaufman's Adaptive Moving Average (KAMA).
*/
type KamaServer struct {
	*runtime.System
	calculator *trend.Kama[float64]
	value      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewKama(ctx context.Context) *KamaServer {
	value := make(chan float64, 1)
	calculator := trend.NewKama[float64]()

	server := &KamaServer{
		System:     runtime.NewSystem(ctx, "financial.trend.kama"),
		calculator: calculator,
		value:      value,
		out:        calculator.ComputeWithContext(ctx, value),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *KamaServer) Write(ctx context.Context, call Kama_write) error {
	valueVal := call.Args().Value()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.value <- valueVal:
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
					"[financial.trend.kama.Write] calculator channel closed",
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
func (server *KamaServer) Done(ctx context.Context, call Kama_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.kama.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
