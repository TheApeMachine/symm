package volume

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volume"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MfvServer calculates the Money Flow Volume (MFV).
*/
type MfvServer struct {
	*runtime.System
	calculator *indicator.Mfv[float64]
	high chan float64
	low chan float64
	close chan float64
	volume chan float64
	out <-chan float64
	result float64
	count int
}

func NewMfv(ctx context.Context) *MfvServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	volume := make(chan float64, 1)
	calculator := indicator.NewMfv[float64]()

	server := &MfvServer{
		System: runtime.NewSystem(ctx, "financial.volume.mfv"),
		calculator: calculator,
		high: high,
		low: low,
		close: close,
		volume: volume,
		out: calculator.ComputeWithContext(ctx, high, low, close, volume),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *MfvServer) Write(ctx context.Context, call Mfv_write) error {
	highVal := call.Args().High()
	lowVal := call.Args().Low()
	closeVal := call.Args().Close()
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
					"[financial.volume.mfv.Write] calculator channel closed",
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
func (server *MfvServer) Done(ctx context.Context, call Mfv_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volume.mfv.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
