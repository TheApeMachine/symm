package volatility

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volatility"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
PercentBServer calculates the Percent B (%B).
*/
type PercentBServer struct {
	*runtime.System
	calculator *indicator.PercentB[float64]
	close      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewPercentB(ctx context.Context) *PercentBServer {
	close := make(chan float64, 1)
	calculator := indicator.NewPercentB[float64]()

	server := &PercentBServer{
		System:     runtime.NewSystem(ctx, "financial.volatility.percent_b"),
		calculator: calculator,
		close:      close,
		out:        calculator.ComputeWithContext(ctx, close),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *PercentBServer) Write(ctx context.Context, call PercentB_write) error {
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
					"[financial.volatility.percent_b.Write] calculator channel closed",
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
func (server *PercentBServer) Done(ctx context.Context, call PercentB_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volatility.percent_b.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
