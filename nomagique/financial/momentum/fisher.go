package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
FisherServer calculates the Fisher Transform.
*/
type FisherServer struct {
	*runtime.System
	calculator *indicator.Fisher[float64]
	close chan float64
	out <-chan float64
	result float64
	count int
}

func NewFisher(ctx context.Context) *FisherServer {
	close := make(chan float64, 1)
	calculator := indicator.NewFisher[float64]()

	server := &FisherServer{
		System: runtime.NewSystem(ctx, "financial.momentum.fisher"),
		calculator: calculator,
		close: close,
		out: calculator.ComputeWithContext(ctx, close),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *FisherServer) Write(ctx context.Context, call Fisher_write) error {
	closeVal := call.Args().Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.close <- closeVal:
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
					"[financial.momentum.fisher.Write] calculator channel closed",
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
func (server *FisherServer) Done(ctx context.Context, call Fisher_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.fisher.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
