package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MlsServer calculates Moving Least Squares slope (m) and intercept (b).
*/
type MlsServer struct {
	*runtime.System
	calculator *trend.Mls[float64]
	x          chan float64
	y          chan float64
	mOut       <-chan float64
	bOut       <-chan float64
	m          float64
	b          float64
	count      int
}

func NewMls(ctx context.Context) *MlsServer {
	x := make(chan float64, 1)
	y := make(chan float64, 1)
	calculator := trend.NewMls[float64]()

	server := &MlsServer{
		System:     runtime.NewSystem(ctx, "financial.trend.mls"),
		calculator: calculator,
		x:          x,
		y:          y,
	}

	server.mOut, server.bOut = calculator.ComputeWithContext(ctx, x, y)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *MlsServer) Write(ctx context.Context, call Mls_write) error {
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
		readFirst := false
		readSecond := false

		for !readFirst || !readSecond {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.mOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.mls.Write] m channel closed",
						nil,
					))
				}

				server.m = res
				readFirst = true
			case res, ok := <-server.bOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.mls.Write] b channel closed",
						nil,
					))
				}

				server.b = res
				readSecond = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *MlsServer) Done(ctx context.Context, call Mls_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.mls.Done] failed to allocate done results",
			err,
		))
	}

	results.SetM(server.m)
	results.SetB(server.b)
	return nil
}
