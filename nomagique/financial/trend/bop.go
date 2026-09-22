package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
BoPServer calculates the balance of power (BoP).
*/
type BoPServer struct {
	*runtime.System
	calculator *trend.Bop[float64]
	opening    chan float64
	high       chan float64
	low        chan float64
	closing    chan float64
	out        <-chan float64
	result     float64
}

func NewBoP(ctx context.Context) *BoPServer {
	opening := make(chan float64, 1)
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	closing := make(chan float64, 1)
	calculator := trend.NewBop[float64]()

	server := &BoPServer{
		System:     runtime.NewSystem(ctx, "financial.trend.bop"),
		calculator: calculator,
		opening:    opening,
		high:       high,
		low:        low,
		closing:    closing,
		out:        calculator.ComputeWithContext(ctx, opening, high, low, closing),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write accepts opening, high, low, and close values.
*/
func (server *BoPServer) Write(ctx context.Context, call BoP_write) error {
	opening := call.Args().Opening()
	high := call.Args().High()
	low := call.Args().Low()
	closeVal := call.Args().Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.opening <- opening:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.high <- high:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.low <- low:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.closing <- closeVal:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res, ok := <-server.out:
		if !ok {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[financial.trend.bop.Write] calculator channel closed",
				nil,
			))
		}

		server.result = res
	}

	return nil
}

/*
Done returns the calculated BoP result.
*/
func (server *BoPServer) Done(ctx context.Context, call BoP_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.bop.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
