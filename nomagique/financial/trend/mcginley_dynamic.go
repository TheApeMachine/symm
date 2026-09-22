package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
McGinleyDynamicServer calculates the McGinley Dynamic indicator.
*/
type McGinleyDynamicServer struct {
	*runtime.System
	calculator *trend.McGinleyDynamic[float64]
	value chan float64
	out <-chan float64
	result float64
	count int
}

func NewMcGinleyDynamic(ctx context.Context) *McGinleyDynamicServer {
	value := make(chan float64, 1)
	calculator := trend.NewMcGinleyDynamic[float64]()

	server := &McGinleyDynamicServer{
		System: runtime.NewSystem(ctx, "financial.trend.mcginley_dynamic"),
		calculator: calculator,
		value: value,
		out: calculator.ComputeWithContext(ctx, value),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *McGinleyDynamicServer) Write(ctx context.Context, call McGinleyDynamic_write) error {
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
					"[financial.trend.mcginley_dynamic.Write] calculator channel closed",
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
func (server *McGinleyDynamicServer) Done(ctx context.Context, call McGinleyDynamic_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.mcginley_dynamic.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
