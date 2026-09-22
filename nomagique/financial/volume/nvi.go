package volume

import (
	"context"

	indicator "github.com/cinar/indicator/v2/volume"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
NviServer calculates the Negative Volume Index (NVI).
*/
type NviServer struct {
	*runtime.System
	calculator *indicator.Nvi[float64]
	close chan float64
	volume chan float64
	out <-chan float64
	result float64
	count int
}

func NewNvi(ctx context.Context) *NviServer {
	close := make(chan float64, 1)
	volume := make(chan float64, 1)
	calculator := indicator.NewNvi[float64]()

	server := &NviServer{
		System: runtime.NewSystem(ctx, "financial.volume.nvi"),
		calculator: calculator,
		close: close,
		volume: volume,
		out: calculator.ComputeWithContext(ctx, close, volume),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *NviServer) Write(ctx context.Context, call Nvi_write) error {
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
					"[financial.volume.nvi.Write] calculator channel closed",
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
func (server *NviServer) Done(ctx context.Context, call Nvi_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.volume.nvi.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
