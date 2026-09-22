package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MassIndexServer calculates the Mass Index.
*/
type MassIndexServer struct {
	*runtime.System
	calculator *trend.MassIndex[float64]
	high       chan float64
	low        chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewMassIndex(ctx context.Context) *MassIndexServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	calculator := trend.NewMassIndex[float64]()

	server := &MassIndexServer{
		System:     runtime.NewSystem(ctx, "financial.trend.mass_index"),
		calculator: calculator,
		high:       high,
		low:        low,
		out:        calculator.ComputeWithContext(ctx, high, low),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *MassIndexServer) Write(ctx context.Context, call MassIndex_write) error {
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
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res, ok := <-server.out:
			if !ok {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"[financial.trend.mass_index.Write] calculator channel closed",
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
func (server *MassIndexServer) Done(ctx context.Context, call MassIndex_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.mass_index.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
