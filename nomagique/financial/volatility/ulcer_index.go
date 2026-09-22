package volatility

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volatility"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
UlcerIndexServer calculates the Ulcer Index (UI).
*/
type UlcerIndexServer struct {
	*runtime.System
	calculator *indicator.UlcerIndex[float64]
	close      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewUlcerIndex(ctx context.Context) *UlcerIndexServer {
	close := make(chan float64, 1)
	calculator := indicator.NewUlcerIndex[float64]()

	server := &UlcerIndexServer{
		System:     runtime.NewSystem(ctx, "financial.volatility.ulcer_index"),
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
func (server *UlcerIndexServer) Write(ctx context.Context, call UlcerIndex_write) error {
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
					"[financial.volatility.ulcer_index.Write] calculator channel closed",
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
func (server *UlcerIndexServer) Done(ctx context.Context, call UlcerIndex_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volatility.ulcer_index.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
