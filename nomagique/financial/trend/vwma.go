package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
VwmaServer calculates the Volume Weighted Moving Average (VWMA).
*/
type VwmaServer struct {
	*runtime.System
	calculator *trend.Vwma[float64]
	close      chan float64
	volume     chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewVwma(ctx context.Context) *VwmaServer {
	close := make(chan float64, 1)
	volume := make(chan float64, 1)
	calculator := trend.NewVwma[float64]()

	server := &VwmaServer{
		System:     runtime.NewSystem(ctx, "financial.trend.vwma"),
		calculator: calculator,
		close:      close,
		volume:     volume,
		out:        calculator.ComputeWithContext(ctx, close, volume),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *VwmaServer) Write(ctx context.Context, call Vwma_write) error {
	closeVal := call.Args().Close()
	volumeVal := call.Args().Volume()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.close <- closeVal:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.volume <- volumeVal:
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
					"[financial.trend.vwma.Write] calculator channel closed",
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
func (server *VwmaServer) Done(ctx context.Context, call Vwma_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.vwma.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
