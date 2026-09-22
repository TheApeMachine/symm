package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
QstickServer calculates the Qstick indicator.
*/
type QstickServer struct {
	*runtime.System
	calculator *indicator.Qstick[float64]
	open chan float64
	close chan float64
	out <-chan float64
	result float64
	count int
}

func NewQstick(ctx context.Context) *QstickServer {
	open := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := indicator.NewQstick[float64]()

	server := &QstickServer{
		System: runtime.NewSystem(ctx, "financial.momentum.qstick"),
		calculator: calculator,
		open: open,
		close: close,
		out: calculator.ComputeWithContext(ctx, open, close),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *QstickServer) Write(ctx context.Context, call Qstick_write) error {
	openVal := call.Args().Open()
	closeVal := call.Args().Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.open <- openVal:
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
					"[financial.momentum.qstick.Write] calculator channel closed",
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
func (server *QstickServer) Done(ctx context.Context, call Qstick_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.qstick.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
