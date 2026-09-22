package volume

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volume"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
EmvServer calculates the Ease of Movement (EMV).
*/
type EmvServer struct {
	*runtime.System
	calculator *indicator.Emv[float64]
	high chan float64
	low chan float64
	volume chan float64
	out <-chan float64
	result float64
	count int
}

func NewEmv(ctx context.Context) *EmvServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	volume := make(chan float64, 1)
	calculator := indicator.NewEmv[float64]()

	server := &EmvServer{
		System: runtime.NewSystem(ctx, "financial.volume.emv"),
		calculator: calculator,
		high: high,
		low: low,
		volume: volume,
		out: calculator.ComputeWithContext(ctx, high, low, volume),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *EmvServer) Write(ctx context.Context, call Emv_write) error {
	highVal := call.Args().High()
	lowVal := call.Args().Low()
	volumeVal := call.Args().Volume()

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
					"[financial.volume.emv.Write] calculator channel closed",
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
func (server *EmvServer) Done(ctx context.Context, call Emv_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volume.emv.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
