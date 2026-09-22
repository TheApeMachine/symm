package statistic

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
LinearRegressionServer calculates online least-squares linear regression parameters alpha and beta.
*/
type LinearRegressionServer struct {
	*runtime.System
	count float64
	meanX float64
	meanY float64
	m2X float64
	coMoment float64
	alpha float64
	beta float64
}

func NewLinearRegression(ctx context.Context) *LinearRegressionServer {
	server := &LinearRegressionServer{
		System: runtime.NewSystem(ctx, "statistic.linear_regression"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *LinearRegressionServer) Write(ctx context.Context, call LinearRegression_write) error {
	xVal := call.Args().X()
	yVal := call.Args().Y()
	server.count++
	deltaX := xVal - server.meanX
	server.meanX += deltaX / server.count
	deltaY := yVal - server.meanY
	server.meanY += deltaY / server.count
	server.m2X += deltaX * (xVal - server.meanX)
	server.coMoment += deltaX * (yVal - server.meanY)

	if server.count > 1 && server.m2X > 0 {
		server.beta = server.coMoment / server.m2X
		server.alpha = server.meanY - server.beta * server.meanX
	}
	return nil
}

/*
Done returns calculated results.
*/
func (server *LinearRegressionServer) Done(ctx context.Context, call LinearRegression_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.linear_regression.Done] failed to allocate done results",
			err,
		))
	}

	results.SetAlpha(server.alpha)
	results.SetBeta(server.beta)
	return nil
}
