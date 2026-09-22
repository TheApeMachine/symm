package volume

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volume"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MfmServer calculates the Money Flow Multiplier (MFM).
*/
type MfmServer struct {
	*runtime.System
	calculator *indicator.Mfm[float64]
	high chan float64
	low chan float64
	close chan float64
	out <-chan float64
	result float64
	count int
}

func NewMfm(ctx context.Context) *MfmServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := indicator.NewMfm[float64]()

	server := &MfmServer{
		System: runtime.NewSystem(ctx, "financial.volume.mfm"),
		calculator: calculator,
		high: high,
		low: low,
		close: close,
		out: calculator.ComputeWithContext(ctx, high, low, close),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *MfmServer) Write(ctx context.Context, call Mfm_write) error {
	highVal := call.Args().High()
	lowVal := call.Args().Low()
	closeVal := call.Args().Close()

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

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res, ok := <-server.out:
			if !ok {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"[financial.volume.mfm.Write] calculator channel closed",
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
func (server *MfmServer) Done(ctx context.Context, call Mfm_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volume.mfm.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
