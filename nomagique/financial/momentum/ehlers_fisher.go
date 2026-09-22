package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
EhlersFisherServer calculates the Ehlers Fisher Transform.
*/
type EhlersFisherServer struct {
	*runtime.System
	calculator *indicator.EhlersFisher[float64]
	high chan float64
	low chan float64
	out <-chan float64
	result float64
	count int
}

func NewEhlersFisher(ctx context.Context) *EhlersFisherServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	calculator := indicator.NewEhlersFisher[float64]()

	server := &EhlersFisherServer{
		System: runtime.NewSystem(ctx, "financial.momentum.ehlers_fisher"),
		calculator: calculator,
		high: high,
		low: low,
		out: calculator.ComputeWithContext(ctx, high, low),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *EhlersFisherServer) Write(ctx context.Context, call EhlersFisher_write) error {
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
					"[financial.momentum.ehlers_fisher.Write] calculator channel closed",
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
func (server *EhlersFisherServer) Done(ctx context.Context, call EhlersFisher_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.ehlers_fisher.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
