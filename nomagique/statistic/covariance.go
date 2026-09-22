package statistic

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
CovarianceServer calculates online sample covariance between two streaming variables.
*/
type CovarianceServer struct {
	*runtime.System
	count float64
	meanX float64
	meanY float64
	m2X float64
	m2Y float64
	coMoment float64
	result float64
}

func NewCovariance(ctx context.Context) *CovarianceServer {
	server := &CovarianceServer{
		System: runtime.NewSystem(ctx, "statistic.covariance"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *CovarianceServer) Write(ctx context.Context, call Covariance_write) error {
	xVal := call.Args().X()
	yVal := call.Args().Y()
	server.count++
	deltaX := xVal - server.meanX
	server.meanX += deltaX / server.count
	deltaY := yVal - server.meanY
	server.meanY += deltaY / server.count
	server.m2X += deltaX * (xVal - server.meanX)
	server.m2Y += deltaY * (yVal - server.meanY)
	server.coMoment += deltaX * (yVal - server.meanY)

	if server.count > 1 {
		server.result = server.coMoment / (server.count - 1)
	}
	return nil
}

/*
Done returns calculated results.
*/
func (server *CovarianceServer) Done(ctx context.Context, call Covariance_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.covariance.Done] failed to allocate done results",
			err,
		))
	}

	results.SetCovariance(server.result)
	return nil
}
