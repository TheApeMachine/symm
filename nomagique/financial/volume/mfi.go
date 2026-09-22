package volume

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volume"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MfiServer calculates the Money Flow Index (MFI).
*/
type MfiServer struct {
	*runtime.System
	calculator *indicator.Mfi[float64]
	high       chan float64
	low        chan float64
	close      chan float64
	volume     chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewMfi(ctx context.Context) *MfiServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	volume := make(chan float64, 1)
	calculator := indicator.NewMfi[float64]()

	server := &MfiServer{
		System:     runtime.NewSystem(ctx, "financial.volume.mfi"),
		calculator: calculator,
		high:       high,
		low:        low,
		close:      close,
		volume:     volume,
		out:        calculator.ComputeWithContext(ctx, high, low, close, volume),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *MfiServer) Write(ctx context.Context, call Mfi_write) error {
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
					"[financial.volume.mfi.Write] calculator channel closed",
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
func (server *MfiServer) Done(ctx context.Context, call Mfi_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volume.mfi.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
