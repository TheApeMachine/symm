package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
BoPServer calculates balance of power,
*/
type BoPServer struct {
	*runtime.System
	calculator *trend.Bop[float64]
	opening    chan float64
	high       chan float64
	low        chan float64
	close      chan float64
	result     float64
}

func NewBoPServer(ctx context.Context) *BoPServer {
	return &BoPServer{
		System:     runtime.NewSystem(ctx, "financial.bop"),
		calculator: trend.NewBop[float64](),
	}
}

/*
Write accepts opening, high, low, and close.
*/
func (server *BoPServer) Write(ctx context.Context, call BoPServer_write) error {
	opening, err := call.Args().Opening()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[financial.bop.Calculate] opening argument is required",
			err,
		))
	}

	server.opening <- opening

	high, err := call.Args().High()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[financial.bop.Calculate] high argument is required",
			err,
		))
	}

	server.high <- high

	low, err := call.Args().Low()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[financial.bop.Calculate] low argument is required",
			err,
		))
	}

	server.low <- low

	close, err := call.Args().Close()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[financial.bop.Calculate] 	opening argument is required",
			err,
		))
	}

	server.close <- close

	server.result = <-server.calculator.ComputeWithContext(
		server.Context(),
		server.opening,
		server.high,
		server.low,
		server.close,
	)

	return nil
}

/*
Done returns the result.
*/
func (server *BoPServer) Done(ctx context.Context, call BoPServer_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.bop] failed to allocate done results",
			err,
		))
	}

	return results.SetResult(server.result)
}
