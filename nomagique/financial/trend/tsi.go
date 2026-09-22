package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
TsiServer calculates the True Strength Index (TSI).
*/
type TsiServer struct {
	*runtime.System
	calculator *trend.Tsi[float64]
	value      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewTsi(ctx context.Context) *TsiServer {
	value := make(chan float64, 1)
	calculator := trend.NewTsi[float64]()

	server := &TsiServer{
		System:     runtime.NewSystem(ctx, "financial.trend.tsi"),
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
func (server *TsiServer) Write(ctx context.Context, call Tsi_write) error {
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
					"[financial.trend.tsi.Write] calculator channel closed",
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
func (server *TsiServer) Done(ctx context.Context, call Tsi_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.tsi.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
