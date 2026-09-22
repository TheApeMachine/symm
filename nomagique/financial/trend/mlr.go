package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MlrServer calculates the Moving Linear Regression (MLR).
*/
type MlrServer struct {
	*runtime.System
	calculator *trend.Mlr[float64]
	x          chan float64
	y          chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewMlr(ctx context.Context) *MlrServer {
	x := make(chan float64, 1)
	y := make(chan float64, 1)
	calculator := trend.NewMlr[float64]()

	server := &MlrServer{
		System:     runtime.NewSystem(ctx, "financial.trend.mlr"),
		calculator: calculator,
		x:          x,
		y:          y,
		out:        calculator.ComputeWithContext(ctx, x, y),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *MlrServer) Write(ctx context.Context, call Mlr_write) error {
	xVal := call.Args().X()
	yVal := call.Args().Y()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.x <- xVal:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.y <- yVal:
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
					"[financial.trend.mlr.Write] calculator channel closed",
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
func (server *MlrServer) Done(ctx context.Context, call Mlr_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.mlr.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
